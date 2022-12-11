# HalfLife

HalfLife is a Cosmos-based blockchain validator monitoring and alerting utility

Monitors and alerts for scenarios such as:
- Slashing period uptime
- Recent missed blocks (is the validator signing currently)
- Jailed status
- Tombstoned status
- Individual sentry nodes unreachable/out of sync
- Chain halted

Discord messages are created in the configured webhook channel for:
- Current validator status
- Detected alerts

## Quick start

### Download

Binaries are available on the [releases](https://github.com/strangelove-ventures/half-life/releases) page.

### Setup config.yaml

Copy `config.yaml.example` to `config.yaml` and populate with your discord and validator information.
You can optionally provide the `sentries` array to also monitor the sentries via grpc.
`rpc-retries` can optionally be provided to override the default of 5 RPC retries before alerting, useful for congested RPC servers.
`fullnode` can be set to `true` to only monitor reachable and out of sync for the provided `sentries`. `address` is not required when `fullnode` is `true`.
`sentry-grpc-error-threshold` can be provided for each validator to tune how many grpc errors are detected (roughtly 30 seconds between checks) before issuing a notification.

See [here](https://support.discord.com/hc/en-us/articles/228383668-Intro-to-Webhooks) for how to create a webhook for a discord channel.

Once you've created the webhook, copy the URL. It'll look something like this: `https://discord.com/api/webhooks/978129125394247720/cwM4Ks-kWcK3Jsg4I_cboauYjOa48ngI2VKaS76afsMwuY7-U4Frw3BGcYXCJvZJ2kWD`

This will be used later to be put into the config.yaml. The webhook id is `978129125394247720` (from the URL), and webhook token is `cwM4Ks-kWcK3Jsg4I_cboauYjOa48ngI2VKaS76afsMwuY7-U4Frw3BGcYXCJvZJ2kWD`

Save the values as follows (note these values are from the URL):
```yml:
webhook:
  id: 978129125394247720
  token: cwM4Ks-kWcK3Jsg4I_cboauYjOa48ngI2VKaS76afsMwuY7-U4Frw3BGcYXCJvZJ2kWD
```

### Start monitoring

Begin monitoring with:

```bash
halflife monitor
```

By default, `half-life monitor` will look for `config.yaml` in the current working directory. To specify a different config file path, use the `--file`/`-f` flag:

```bash
halflife monitor -f ~/config.yaml
```

When a validator is first added to `config.yaml` and halflife is started, a status message will be created in the discord channel and the ID of that message will be added to `config.yaml`. Pin this message so that the channel's pinned messages can act as a dashboard to see the realtime status of the validators.

![Screenshot from 2022-02-28 14-29-36](https://user-images.githubusercontent.com/6722152/156061805-330d1c76-acfa-4089-b327-f35f686fa0e7.png)

Alerts will be posted when any error conditions are detected, and follow up messages will be posted when those errors are cleared.

![Screenshot from 2022-02-16 10-53-43](https://user-images.githubusercontent.com/6722152/154326098-12aa787f-389e-4abf-af56-93918090ddc1.png)

For high and critical errors, the configured discord user IDs will be tagged

![Screenshot from 2022-02-16 11-38-00](https://user-images.githubusercontent.com/6722152/154333667-af823075-73fc-4d41-97ce-40432f3450ac.png)

## Build from source

### Install Go

Go is necessary in order to build `half-life` from source.

```
# install
wget https://golang.org/dl/go1.18.2.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.18.2.linux-amd64.tar.gz

# source go
cat <<EOF >> ~/.profile
export GOROOT=/usr/local/go
export GOPATH=$HOME/go
export GO111MODULE=on
export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin
EOF
source ~/.profile
go version
```

### Install halflife binary

```
cd ~/half-life
go install
```
