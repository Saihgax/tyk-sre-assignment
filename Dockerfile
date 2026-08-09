FROM golang:1.22-alpine AS builder

WORKDIR /src

COPY golang/go.mod golang/go.sum ./golang/

WORKDIR /src/golang
RUN go mod download

COPY golang/ .

RUN CGO_ENABLED=0 GOOS=linux go build -o /tyk-sre-assignment .

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /tyk-sre-assignment /tyk-sre-assignment

ENTRYPOINT ["/tyk-sre-assignment"]