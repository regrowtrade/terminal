FROM node:24-alpine AS build
WORKDIR /src
ENV NEXT_TELEMETRY_DISABLED=1
COPY package.json package-lock.json ./
COPY apps/web/package.json apps/web/package.json
RUN npm ci
COPY apps/web/ apps/web/
COPY .railway/ .railway/
RUN npm run build

FROM node:24-alpine AS runtime
WORKDIR /app
ENV NODE_ENV=production NEXT_TELEMETRY_DISABLED=1 HOSTNAME=0.0.0.0 PORT=3000
RUN addgroup -g 10001 terminal && adduser -D -u 10001 -G terminal terminal
COPY --from=build --chown=terminal:terminal /src/apps/web/.next/standalone/ ./
COPY --from=build --chown=terminal:terminal /src/apps/web/.next/static/ ./apps/web/.next/static/
USER terminal
EXPOSE 3000
CMD ["node", "apps/web/server.js"]
