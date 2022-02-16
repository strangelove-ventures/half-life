# HalfLife

HalfLife is a Cosmos-based blockchain validator monitoring and alerting utility

Monitors and alerts for scenarios such as:
- Slashing period uptime
- Recent missed blocks (is the validator signing currently)
- Jailed status
- Tombstoned status

Discord messages are created in the configured webhook channel for:
- Current validator status
- Detected alerts

## Quick start

Copy `config.yaml.example` to `config.yaml` and populate with your discord and validator information

Begin monitoring with:
```bash
halflife monitor
```