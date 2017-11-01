# Canary Release

## About the project

this repo provide cananry release for caicloud application

### Status

**Working in process**

This project is still in alpha version

### Design

Learn more about canary release

-   design doc on [google dirve](http://ffff.im/0Klf)
-   api [defination](https://github.com/caicloud/clientset/blob/master/pkg/apis/release/v1alpha1/types.go#L162)](https://github.com/caicloud/clientset/blob/master/pkg/apis/release/v1alpha1/types.go#L162)

## Getting Started

### Layout

```
.
├── controller
│   ├── bin
│   ├── build
│   ├── cmd
│   ├── config
│   └── controller
├── pkg
│   ├── api
│   ├── chart
│   ├── util
│   └── version
└── proxies
    └── nginx
        ├── build
        │   └── rootfs
        ├── cmd
        ├── config
        ├── controller
        └── template
```

A brief description: 

-   `controller` contains a complete controller project
    -   `bin` is to hold build outputs
    -   `build` contains scripts, yaml files, dockerfiels, etc, to build and package the project
    -   `cmd` contains main package
    -   `config` contains controller config
-   `pkg` contains api, chart, util, version
-   `proxies` contains proxy packages, each subdirectory is one kind of proxy
    -   `nginx` contains nginx proxy package