# Build stage compiles the app; runtime stage serves the built assets with
# vite preview (which also proxies /api to the backend). For a hardened
# production deployment, plan to serve dist/ behind a reverse proxy instead.
FROM node:22-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM node:22-alpine
WORKDIR /app
COPY --from=build /app /app
ENV NODE_ENV=production
EXPOSE 4173
CMD ["npm", "run", "preview", "--", "--host", "0.0.0.0"]
