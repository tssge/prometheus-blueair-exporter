FROM golang:alpine

# copy the source code
COPY app /app
WORKDIR /app

# build the exporter
RUN go build -o prometheus-blueair-exporter

# expose the port used by the exporter
EXPOSE 2735

ENTRYPOINT ["/app/prometheus-blueair-exporter"]
