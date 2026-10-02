# syntax=docker/dockerfile:1
FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/metrics-gate .

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/metrics-gate /app/metrics-gate
COPY config.example.yaml /app/config.example.yaml
COPY overlay.example.yaml /app/overlay.example.yaml
# Same relative paths as config.example.yaml
COPY app/index/testdata/manifest.mini.json /app/app/index/testdata/manifest.mini.json
EXPOSE 8080 8090
USER nonroot:nonroot
ENTRYPOINT ["/app/metrics-gate"]
CMD ["--config", "/app/config.example.yaml"]
