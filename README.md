# Planning Poker

A lightweight, real-time sprint story point estimating tool built with Go and React. Estimate tasks with zero friction—no accounts, no data tracking, just collaborative refinement.

## Features

- **Real-time Collaboration:** Instant updates using WebSockets.
- **Customisable Decks:** Configure your own card sets (Fibonacci, T-shirt sizes, etc.).
- **Interactive Chat:** Integrated room chat for discussing estimates.
- **Privacy First:** No persistent storage or user accounts required.
- **Participation Roles:** Join as a Participant to vote or an Observer to facilitate.

## Tech Stack

- **Backend:** Go 1.23, Gorilla WebSocket
- **Frontend:** React, TypeScript, Vite, Bootstrap
- **Deployment:** Docker

## Getting Started

### Prerequisites

- [Docker](https://docs.docker.com/get-docker/)

### Build and Run with Docker

1. **Build the image:**

   ```bash
   docker buildx build -t planning-poker .
   ```

2. **Run the container:**

   ```bash
   docker run --rm -it -p 8093:8080 planning-poker
   ```

3. **Access the app:**

   Open your browser and navigate to [http://localhost:8093](http://localhost:8093).

## Development

If you want to run the components separately:

### Backend

```bash
go run cmd/server/main.go
```
The server will start on port 8080. Note that it expects the frontend to be built in `ui/dist` to serve static files.

### Frontend

```bash
cd ui
npm install
npm run dev
```
The development server will start on port 5173. You may need to configure a proxy or adjust the WebSocket URL for local development if not using the Go server's bundled distribution.
