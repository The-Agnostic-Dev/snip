# syntax=docker/dockerfile:1

FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/snip ./cmd/snip

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/snip /snip
ENV PORT=5110
EXPOSE 5110
ENTRYPOINT ["/snip"]
