# slack-oc-bot
Slack Bot to execute oc (and kubectl) commands

## Usage

This is the way how you can run this Slack bot application;

```shell
$ podman run -rm -e "SLACK_APP_TOKEN=$SLACK_APP_TOKEN" -e "SLACK_AUTH_TOKEN=$SLACK_AUTH_TOKEN" quay.io/aguclu/slack-oc-bot
```
