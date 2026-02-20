# Stage 1: Build the React frontend
FROM node:20-alpine AS ui-build
WORKDIR /ui
COPY ui/package*.json ./
RUN npm install
COPY ui/ ./
RUN npm run build

# Stage 2: Build the Go backend
FROM golang:1.23-alpine AS server-build
WORKDIR /app
COPY . .
RUN go mod tidy
RUN go build -o poker-server cmd/server/main.go

# Stage 3: Final minimal image
FROM alpine:latest
RUN apk add --no-cache tzdata
ENV TZ=Australia/Sydney
WORKDIR /root/
COPY --from=server-build /app/poker-server .
COPY --from=ui-build /ui/dist ./ui/dist

EXPOSE 8080
CMD ["./poker-server"]
