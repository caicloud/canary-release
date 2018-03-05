# Canary Release

## About the project

This project implements a canary release system based on [Rudder](https://github.com/caicloud/rudder)

### Status

The project is still in `alpha` version

### Design

Learn more about canary release on [design doc](docs/design.md)

## Getting Started

### Layout

```
├── docs
├── hack
├── build
│   ├── controller
│   ├── nginx-base
│   └── nginx-proxy
│       ├── controller
│       └── etc
├── cmd
│   ├── controller
│   └── nginx-proxy
├── controller
│   ├── bin
│   ├── config
│   └── controller
└── proxies
    └── nginx
├── pkg
│   ├── api
│   ├── chart
│   ├── util
│   └── version
```

Explanation for main pkgs:
- `build` contains dockerfiles for canary release.
- `cmd` contains main packags, each subdirectory of cmd is a main package.
- `docs` for project documentations.
- `controller` contains codes for canary release controller 
- `proxies` contains canary release proxies, each subdirectory is a kind of proxies.
- `pkg` contains utilities for canary release.
