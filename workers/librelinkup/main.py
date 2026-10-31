#!/usr/bin/env python3
"""Push LibreLinkUp follower readings to glucose-dashboard.

Abbott's LibreLinkUp API is unofficial and can change without notice.
"""

from __future__ import annotations

import logging
import math
import os
import signal
import sys
import time
from datetime import UTC, datetime

import requests
from pylibrelinkup import (
    APIUrl,
    EmailVerificationError,
    LLUAPIRateLimitError,
    PatientNotFoundError,
    PrivacyPolicyError,
    PyLibreLinkUp,
    RedirectError,
    TermsOfUseError,
)

log = logging.getLogger("librelinkup")

SOURCE = "librelinkup"
BATCH = 2000
STOP = False


class DashboardAuthError(Exception):
    """The dashboard rejected the API key."""


class DashboardClientError(Exception):
    """The dashboard rejected the payload. Retrying the same body will not help this cycle."""


class NoConnectionError(Exception):
    """The follower account has no connections."""


class PatientNotConfiguredError(Exception):
    """LLU_PATIENT_ID did not match a connection."""


class Config:
    def __init__(
        self,
        server_url: str,
        api_key: str,
        email: str,
        password: str,
        region: APIUrl,
        patient_id: str,
        poll_interval: float,
    ) -> None:
        self.server_url = server_url
        self.api_key = api_key
        self.email = email
        self.password = password
        self.region = region
        self.patient_id = patient_id
        self.poll_interval = poll_interval

    @classmethod
    def from_env(cls) -> Config:
        region_name = os.environ.get("LLU_REGION", "").strip()
        try:
            region = APIUrl.from_string(region_name) if region_name else APIUrl.US
        except ValueError:
            log.error("LLU_REGION is not a known region")
            sys.exit(1)

        poll_raw = os.environ.get("POLL_INTERVAL", "").strip() or "60"
        try:
            poll = float(poll_raw)
        except ValueError:
            log.error("POLL_INTERVAL must be a number of seconds")
            sys.exit(1)
        if poll < 1:
            log.error("POLL_INTERVAL must be at least 1")
            sys.exit(1)

        return cls(
            server_url=require("SERVER_URL").rstrip("/"),
            api_key=require("API_KEY"),
            email=require("LLU_EMAIL"),
            password=require("LLU_PASSWORD", strip=False),
            region=region,
            patient_id=os.environ.get("LLU_PATIENT_ID", "").strip(),
            poll_interval=poll,
        )


class LibreSession:
    def __init__(self, email: str, password: str, region: APIUrl) -> None:
        self.email = email
        self.password = password
        self.region = region
        self.client: PyLibreLinkUp | None = None

    def authenticate(self) -> None:
        client = self._new_client(self.region)
        try:
            client.authenticate()
        except RedirectError as exc:
            self.region = exc.region
            log.info("LibreLinkUp region redirect to %s", exc.region.name)
            client = self._new_client(self.region)
            client.authenticate()
        self.client = client

    def call(self, fn):
        if self.client is None:
            self.authenticate()
        try:
            return fn(self.client)
        except requests.HTTPError as exc:
            status = exc.response.status_code if exc.response is not None else None
            if status != 401:
                raise
            log.info("LibreLinkUp session expired; signing in again")
            self.authenticate()
            return fn(self.client)

    def readings(self, patient, methods: tuple[str, ...]) -> list[dict]:
        combined = []
        for name in methods:
            rows = self.call(lambda client, method=name: getattr(client, method)(patient_identifier=patient))
            if rows:
                combined.extend(rows)
        return measurements_to_readings(combined)

    def _new_client(self, region: APIUrl) -> PyLibreLinkUp:
        return PyLibreLinkUp(email=self.email, password=self.password, api_url=region)


def main() -> None:
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
    signal.signal(signal.SIGTERM, request_stop)
    signal.signal(signal.SIGINT, request_stop)

    cfg = Config.from_env()
    session = LibreSession(cfg.email, cfg.password, cfg.region)
    http = requests.Session()
    backfilled = False
    patient = None
    attempt = 0

    while not STOP:
        try:
            if patient is None:
                patient = session.call(lambda client: select_patient(client, cfg.patient_id))
            if not backfilled:
                readings = session.readings(patient, ("logbook", "graph"))
                post_readings(http, cfg.server_url, cfg.api_key, readings)
                backfilled = True
                log.info("backfill complete count=%s", len(readings))
            else:
                readings = session.readings(patient, ("graph",))
                post_readings(http, cfg.server_url, cfg.api_key, readings)
            attempt = 0
            if not sleep_for(cfg.poll_interval):
                break
        except KeyboardInterrupt:
            break
        except (TermsOfUseError, PrivacyPolicyError, EmailVerificationError) as exc:
            log.error("LibreLinkUp account needs attention in the app (%s)", type(exc).__name__)
            attempt += 1
            if not sleep_for(backoff(attempt)):
                break
        except (NoConnectionError, PatientNotConfiguredError, PatientNotFoundError) as exc:
            log.error("LibreLinkUp connection is unavailable (%s)", type(exc).__name__)
            patient = None
            attempt += 1
            if not sleep_for(backoff(attempt)):
                break
        except DashboardAuthError:
            log.error("dashboard rejected the API key")
            attempt += 1
            if not sleep_for(backoff(attempt)):
                break
        except LLUAPIRateLimitError as exc:
            delay = exc.retry_after if isinstance(getattr(exc, "retry_after", None), int) else None
            attempt += 1
            if delay is None or delay <= 0:
                delay = backoff(attempt)
            log.warning("LibreLinkUp rate limited; sleeping %s seconds", delay)
            if not sleep_for(float(delay)):
                break
        except requests.HTTPError as exc:
            status = exc.response.status_code if exc.response is not None else "unknown"
            log.error("request failed status=%s", status)
            attempt += 1
            if not sleep_for(backoff(attempt)):
                break
        except requests.RequestException:
            log.error("network error")
            attempt += 1
            if not sleep_for(backoff(attempt)):
                break
        except Exception as exc:
            log.error("cycle failed: %s", type(exc).__name__)
            attempt += 1
            if not sleep_for(backoff(attempt)):
                break


def select_patient(client: PyLibreLinkUp, patient_id: str):
    patients = client.get_patients()
    if not patients:
        raise NoConnectionError()
    if not patient_id:
        log.info("using the first LibreLinkUp connection")
        return patients[0]
    for patient in patients:
        if str(patient.patient_id) == patient_id:
            log.info("using the configured LibreLinkUp connection")
            return patient
    raise PatientNotConfiguredError()


def measurements_to_readings(measurements) -> list[dict]:
    ordered: dict[str, int] = {}
    for measurement in measurements:
        factory = getattr(measurement, "factory_timestamp", None)
        if not isinstance(factory, datetime):
            continue
        if factory.tzinfo is None:
            factory = factory.replace(tzinfo=UTC)
        factory = factory.astimezone(UTC).replace(second=0, microsecond=0)
        key = factory.strftime("%Y-%m-%dT%H:%M:%SZ")
        try:
            mgdl = int(math.floor(float(measurement.value_in_mg_per_dl) + 0.5))
        except (TypeError, ValueError, AttributeError):
            continue
        ordered[key] = mgdl
    return [{"ts": ts, "mgdl": mgdl} for ts, mgdl in ordered.items()]


def post_readings(http: requests.Session, server_url: str, api_key: str, readings: list[dict]) -> None:
    if not readings:
        log.info("no readings to send")
        return
    endpoint = server_url + "/api/v1/readings"
    for start in range(0, len(readings), BATCH):
        chunk = readings[start : start + BATCH]
        response = http.post(
            endpoint,
            json={"source": SOURCE, "readings": chunk},
            headers={"Authorization": f"Bearer {api_key}"},
            timeout=30,
        )
        if response.status_code == 401:
            raise DashboardAuthError()
        if response.status_code >= 500:
            raise requests.HTTPError(response=response)
        if response.status_code >= 400:
            log.error("dashboard rejected batch status=%s", response.status_code)
            raise DashboardClientError()
        body: dict = {}
        try:
            parsed = response.json()
            if isinstance(parsed, dict):
                body = parsed
        except ValueError:
            body = {}
        rejected = body.get("rejected") or []
        log.info(
            "posted readings received=%s inserted=%s updated=%s rejected=%s",
            body.get("received"),
            body.get("inserted"),
            body.get("updated"),
            len(rejected) if isinstance(rejected, list) else "unknown",
        )


def require(name: str, strip: bool = True) -> str:
    value = os.environ.get(name, "")
    if strip:
        value = value.strip()
    if value.strip() == "":
        log.error("missing required environment variable %s", name)
        sys.exit(1)
    return value


def backoff(attempt: int) -> float:
    return float(min(300, 2 ** min(attempt, 8)))


def sleep_for(seconds: float) -> bool:
    deadline = time.monotonic() + seconds
    while not STOP:
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            return True
        time.sleep(min(1.0, remaining))
    return False


def request_stop(_signum, _frame) -> None:
    global STOP
    STOP = True
    log.info("stop requested")


if __name__ == "__main__":
    main()
