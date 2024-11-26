![Baton Logo](./docs/images/baton-logo.png)

# `baton-bitbucket` [![Go Reference](https://pkg.go.dev/badge/github.com/conductorone/baton-bitbucket.svg)](https://pkg.go.dev/github.com/conductorone/baton-bitbucket) ![main ci](https://github.com/conductorone/baton-bitbucket/actions/workflows/main.yaml/badge.svg)

`baton-bitbucket` is a connector for Bitbucket built using the [Baton SDK](https://github.com/conductorone/baton-sdk). It communicates with the Bitbucket User provisioning API to sync data about workspaces, user groups, users, projects and their repositories.

Check out [Baton](https://github.com/conductorone/baton) to learn more about the project in general.

# Prerequisites

To work with the connector, you can choose from multiple authentication methods. You can either use an application password with login username and generated password, an API access token, or a consumer key and secret for oauth flow.

Each one of these methods are configurable with permissions (Read, Write, Admin) to access the Bitbucket API. The permissions required for this connector are:
- Read: `Workspace`, `UserGroup`, `User`, `Project`, `Repository`
- Admin: `Project`, `Repository`

Mentioned auth methods like API Access Tokens can be scoped to different resources, and the connector only allows the workspace-scoped token or the user-scoped password with required permissions described above.

# Getting Started

## brew

```
brew install conductorone/baton/baton conductorone/baton/baton-bitbucket

BATON_USERNAME=username BATON_PASSWORD=password baton-bitbucket
baton resources
```

## docker

```
docker run --rm -v $(pwd):/out -e BATON_TOKEN=token ghcr.io/conductorone/baton-bitbucket:latest -f "/out/sync.c1z"
docker run --rm -v $(pwd):/out ghcr.io/conductorone/baton:latest -f "/out/sync.c1z" resources
```

## source

```
go install github.com/conductorone/baton/cmd/baton@main
go install github.com/conductorone/baton-bitbucket/cmd/baton-bitbucket@main

BATON_CONSUMER_ID=consumerKey BATON_CONSUMER_SECRET=consumerSecret baton-bitbucket
baton resources
```

# Data Model

`baton-bitbucket` will pull down information about the following Bitbucket resources:

- Workspaces
- UserGroups
- Users
- Projects
- Repositories

By default, `baton-bitbucket` will sync information from workspaces based on provided credential. You can specify exactly which workspaces you would like to sync using the `--workspaces` flag.

# Contributing, Support and Issues

We started Baton because we were tired of taking screenshots and manually building spreadsheets. We welcome contributions, and ideas, no matter how small -- our goal is to make identity and permissions sprawl less painful for everyone. If you have questions, problems, or ideas: Please open a Github Issue!

See [CONTRIBUTING.md](https://github.com/ConductorOne/baton/blob/main/CONTRIBUTING.md) for more details.

# `baton-bitbucket` Command Line Usage

```
baton-bitbucket

Usage:
  baton-bitbucket [flags]
  baton-bitbucket [command]

Available Commands:
  capabilities       Get connector capabilities
  completion         Generate the autocompletion script for the specified shell
  help               Help about any command

Flags:
      --app-password string      Application password used to connect to the BitBucket API. ($BATON_APP_PASSWORD)
      --client-id string         The client ID used to authenticate with ConductorOne ($BATON_CLIENT_ID)
      --client-secret string     The client secret used to authenticate with ConductorOne ($BATON_CLIENT_SECRET)
      --consumer-key string      OAuth consumer key used to connect to the BitBucket API via oauth. ($BATON_CONSUMER_KEY)
      --consumer-secret string   The consumer secret used to connect to the BitBucket API via oauth. ($BATON_CONSUMER_SECRET)
  -f, --file string              The path to the c1z file to sync with ($BATON_FILE) (default "sync.c1z")
  -h, --help                     help for baton-bitbucket
      --log-format string        The output format for logs: json, console ($BATON_LOG_FORMAT) (default "json")
      --log-level string         The log level: debug, info, warn, error ($BATON_LOG_LEVEL) (default "info")
  -p, --provisioning             This must be set in order for provisioning actions to be enabled ($BATON_PROVISIONING)
      --skip-full-sync           This must be set to skip a full sync ($BATON_SKIP_FULL_SYNC)
      --ticketing                This must be set to enable ticketing support ($BATON_TICKETING)
      --token string             Access token (workspace or project scoped) used to connect to the BitBucket API. ($BATON_TOKEN)
      --username string          Username of administrator used to connect to the BitBucket API. ($BATON_USERNAME)
  -v, --version                  version for baton-bitbucket
      --workspaces strings       Limit syncing to specific workspaces by specifying workspace slugs. ($BATON_WORKSPACES)

Use "baton-bitbucket [command] --help" for more information about a command.
```
