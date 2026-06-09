FROM node:22-alpine AS build
WORKDIR /app/web

# Copy web package files first (without the local SDK reference)
COPY clients/web/package*.json ./

# Remove local SDK dependency temporarily, install other deps
RUN sed -i '/@constell\/sdk-js/d' package.json && npm install

# Manually install the SDK and its dependencies
COPY sdk/sdk-js/ /app/web/node_modules/@constell/sdk-js/
RUN cd /app/web/node_modules/@constell/sdk-js && npm install

# Copy web source and build
COPY clients/web/ .
RUN npx vite build

FROM nginx:alpine
COPY --from=build /app/web/dist /usr/share/nginx/html
COPY clients/web/nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
