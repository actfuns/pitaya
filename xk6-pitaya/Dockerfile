# Build the k6 binary with the extension
FROM golang:1.23 as builder

ARG pitaya_revision

RUN go install go.k6.io/xk6/cmd/xk6@latest
# For our example, we'll add support for output of test metrics to InfluxDB v2.
# Feel free to add other extensions using the '--with ...'.
RUN xk6 build \
    --with github.com/actfuns/pitaya/xk6-pitaya/v2@$pitaya_revision  \
    --replace github.com/actfuns/pitaya/v2=github.com/actfuns/pitaya/v2@$pitaya_revision \
    --output /k6

# Use the operator's base image and override the k6 binary
FROM grafana/k6:latest
COPY --from=builder /k6 /usr/bin/k6
