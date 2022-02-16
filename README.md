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

See [here](https://support.discord.com/hc/en-us/articles/228383668-Intro-to-Webhooks) for how to create a webhook for a discord channel.

Begin monitoring with:
```bash
halflife monitor
```

When a validator is first added to `config.yaml` and halflife is started, a status message will be created in the discord channel and the ID of that message will be added to `config.yaml`. Pin this message so that the channel's pinned messages can act as a dashboard to see the realtime status of the validators.

![Screenshot from 2022-02-16 10-48-03](https://user-images.githubusercontent.com/6722152/154325942-3331737f-cc84-4fc8-9970-785c95a9cf1d.png)

Alerts will be posted when any error conditions are detected, and follow up messages will be posted when those errors are cleared.

![Screenshot from 2022-02-16 10-53-43](https://user-images.githubusercontent.com/6722152/154326098-12aa787f-389e-4abf-af56-93918090ddc1.png)
