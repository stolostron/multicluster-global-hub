# Setup instructions and demo

## Prerequisites

See [the common prerequisites for Hub-of-Hubs demos](../README.md#prerequisites).

## Installing Hub-of-Hubs components manually

1.  [Deploy Hub-of-Hubs components](https://github.com/stolostron/hub-of-hubs/blob/main/deploy/README.md) on `hoh`.
1.  [Deploy Hub-of-Hubs-agent components](https://github.com/stolostron/hub-of-hubs/tree/main/deploy#deploying-a-hub-of-hubs-agent) on `hub1` and `hub2`.

## Installing Hub-of-Hubs components by a demo shell script

You can demonstrate the setup itself (it could take up to 30 mintues) by running the shell script below **in this directory**. 
The shell script uses [demo-magic](https://github.com/paxtonhare/demo-magic), you need to press Enter to run commands one by one. 
The redirection of the standard error stream to `/dev/null` is required to suppress throttling warnings that expose the API server URL. Exposing API server URL could be undesirable for security.

```
./run.sh 2> /dev/null
```

## Cleanup

```
./clean.sh
```
