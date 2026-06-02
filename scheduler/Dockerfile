# Build the topology-aware scheduler extender.
#
# Build context must be the repository root so that the pkg/decomp
# replace directive in scheduler/go.mod can resolve.
#
#   docker build -f scheduler/Dockerfile -t topology-aware-scheduler .

FROM golang:1.21-alpine AS builder

WORKDIR /src
COPY pkg/decomp /src/pkg/decomp
COPY scheduler  /src/scheduler

WORKDIR /src/scheduler
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/scheduler ./cmd/scheduler

# -----------------------------------------------------------------------------
FROM gcr.io/distroless/static-debian12

COPY --from=builder /out/scheduler /scheduler
USER nonroot:nonroot
EXPOSE 8090
ENTRYPOINT ["/scheduler"]
