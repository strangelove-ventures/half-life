# Celestia Monitoring Bot

Celestia blockchain validator monitoring and alerting service via telegram

Monitors and alerts for scenarios such as:
- Slashing period uptime
- Recent missed blocks (is the validator signing currently)
- Jailed status
- Tombstoned status
- Chain halted

## Quick start

### Download

```bash
docker pull staking4all/celestia-monitoring-bot
```

### Setup config.yaml

Copy `config.yaml.example` to `config.yaml` and populate with your telegram bot token and default validator configs information.
`rpc_retries` can optionally be provided to override the default of 5 RPC retries before alerting, useful for congested RPC servers.


### Start monitoring

Begin monitoring with:

```bash
cmb monitor
```

By default, `cmb monitor` will look for `config.yaml` in the current working directory. To specify a different config file path, use the `--file`/`-f` flag:

```bash
cmb monitor -f ~/config.yaml
```

## Build from source

### Install Go

Go is necessary in order to build `cmb` from source.

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

### Install cmb binary

```
cd ~/cmb
go install
```
