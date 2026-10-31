FROM node:22-bookworm-slim

WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --omit=dev
COPY index.js convert.js dotenv.js ./
USER node
ENTRYPOINT ["node", "index.js"]
