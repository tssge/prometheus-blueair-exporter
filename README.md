# Prometheus Blueair Exporter

A simple Blueair air purifier metrics exporter for Prometheus. Only the models that use Blueair AWS API are supported.

These include newer models like HealthProtect etc. Please feel free to send a pull request if you have a model that is not supported.

The exporter will export all devices that are on your Blueair account and reachable via the Blueair AWS API.

Do note that this is my first Go project, so the code is probably not the best.

## Exported metrics

```
# HELP blueair_fanspeed Fanspeed (%)
# TYPE blueair_fanspeed gauge
blueair_fanspeed{sensor="Device Name"} 11
# HELP blueair_humidity Relative humidity (%)
# TYPE blueair_humidity gauge
blueair_humidity{sensor="Device Name"} 31
# HELP blueair_pm1 Particulate matter, 1 micron (ug/m^3)
# TYPE blueair_pm1 gauge
blueair_pm1{sensor="Device Name"} 4
# HELP blueair_pm10 Particulate matter, 10 micron (ug/m^3)
# TYPE blueair_pm10 gauge
blueair_pm10{sensor="Device Name"} 6
# HELP blueair_pm25 Particulate matter, 2.5 micron (ug/m^3)
# TYPE blueair_pm25 gauge
blueair_pm25{sensor="Device Name"} 12
# HELP blueair_temperature Temperature (C)
# TYPE blueair_temperature gauge
blueair_temperature{sensor="Device Name"} 23
# HELP blueair_voc Volatile organic compounds (ppb)
# TYPE blueair_voc gauge
blueair_voc{sensor="Device Name"} 133
```

## Usage

Required credentials are the ones used for the official Blueair app.

```
Usage:
  prometheus-blueair-exporter [OPTIONS]

Application Options:
  -a, --address=  Address to listen on (default: 0.0.0.0:2735) [$BLUEAIR_ADDRESS]
  -d, --delay=    Delay between attempts to refresh metrics (default: 300s) [$BLUEAIR_DELAY]
  -e, --email=    Email address for Blueair login [$BLUEAIR_EMAIL]
  -p, --password= Password for Blueair login [$BLUEAIR_PASSWORD]

Help Options:
  -h, --help      Show this help message
```

### Docker

```bash
docker run -it --env BLUEAIR_EMAIL="user@email.host" --env BLUEAIR_PASSWORD='LoginPassword' --name BlueairExporter tssge/prometheus-blueair-exporter:latest
```
### Docker Compose

```yaml
---
version: '3.7'
services:
  blueair-exporter:
    image: tssge/prometheus-blueair-exporter:latest
    container_name: BlueairExporter
    environment:
      - 'BLUEAIR_ADDRESS=0.0.0.0:2735'
      - 'BLUEAIR_PASSWORD=WhatEverYourPasswordIs'
      - 'BLUEAIR_DELAY=300s'
      - 'BLUEAIR_EMAIL=WhatEverYourEmailIs'
    ports:
      - 2735:2735
```

### Manual build

```bash
cd app
go build
./prometheus-blueair-exporter --email "WhatEverYourEmailIs" --password "WhatEverYourPasswordIs"
```

## Grafana dashboard

While there is no official dashboard, you can use the following dashboard as a starting point for similar sensor (Awair): https://gist.github.com/gyng/5afd2756497b5ceffd277ca6509baeb4