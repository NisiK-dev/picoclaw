# ============================================================
# Stage 1: Build the picoclaw binary
# ============================================================
FROM golang:1.25.7-alpine AS builder

RUN apk add --no-cache git make

WORKDIR /src

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN make build

# ============================================================
# Stage 2: Minimal runtime image
# ============================================================
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

# Copy binary
COPY --from=builder /src/build/picoclaw /usr/local/bin/picoclaw

# Create a non-root user (Render roda como non-root por padrão)
RUN adduser -D -s /bin/sh picoclaw

# Create picoclaw home directory e roda onboard como o usuário correto
RUN mkdir -p /home/picoclaw/.picoclaw && \
    chown -R picoclaw:picoclaw /home/picoclaw

# Copiar o config.json explicitamente (garante que exista)
COPY --from=builder /src/workspace/config.json /home/picoclaw/.picoclaw/config.json
RUN chown picoclaw:picoclaw /home/picoclaw/.picoclaw/config.json

# Set home environment variable
ENV HOME=/home/picoclaw

USER picoclaw

ENTRYPOINT ["picoclaw"]
CMD ["gateway"]
