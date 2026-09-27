# Static CGO-free binary; distroless/static brings CA certs and nothing else.
FROM golang:1.27-trixie AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /github-actions-runner-exporter .

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=builder /github-actions-runner-exporter /github-actions-runner-exporter
EXPOSE 9222
ENTRYPOINT ["/github-actions-runner-exporter"]
