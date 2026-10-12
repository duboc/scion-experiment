FROM node:20-alpine

WORKDIR /app

COPY package*.json ./
RUN npm install --production

COPY . .

ENV PORT=8080
ENV GCP_PROJECT=riojucu-sandbox
ENV PSEARCH_SERVING_URL=https://psearch-serving-mhjw244bgq-uc.a.run.app

EXPOSE 8080

CMD ["node", "server.js"]
