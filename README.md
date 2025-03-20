# Installation

## Docker-Images

Unter https://github.com/jkulzer/fib-server/pkgs/container/fib-server können die erstellten Docker-Images eingesehen werden. Dabei ist das neueste Docker-Image immer `ghcr.io/jkulzer/fib-server:latest`

## Manuelle Kompilierung

1. Den Paketmanager Nix installieren (https://nixos.org/download)

2. Die Dev-Umgebung starten
```bash
nix develop
```
Diese Entwicklungsumgebung konfiguriert automatisch externe Dependencies der Software, wie beispielsweise SQLite.

Die folgenden Schritte werden alle in der Nix Shell ausgeführt

3. Dependencies installieren

```bash
go mod tidy
```

4. Die App kompilieren

```bash
go build
```

Daraufhin befindet sich die Binary im Pfad `./fib-server` und kann ausgeführt werden.
