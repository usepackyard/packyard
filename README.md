# Packyard

Open-source, self-hosted private Composer registry for PHP teams.

[![License: AGPL v3](https://img.shields.io/badge/license-AGPL--3.0-blue)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8)](https://go.dev)

Packyard gives you a Composer package registry you can run on your own infrastructure. It ships as a single Go binary with an embedded admin dashboard, organization-scoped tokens, and local or S3-compatible package storage.

## Install

```bash
curl -sSf https://get.packyard.dev/install.sh | sh
```

The installer downloads Packyard and starts an interactive setup for the database, storage, public URL, admin user, and optional service unit.

For manual, unattended, air-gapped, or reverse-proxy setup, see the [installation guide](https://packyard.dev/documentation/installation).

## What You Get

- Composer registry endpoints for private PHP packages
- Organization-scoped install tokens
- SQLite, MySQL, or PostgreSQL
- Local filesystem or S3-compatible storage
- GitHub release sync and an embedded admin dashboard

## Composer Setup

Composer repository URLs include the organization slug:

```json
{
  "repositories": [
    {
      "type": "composer",
      "url": "https://repo.example.com/default"
    }
  ]
}
```

Authenticate with a token generated in the Packyard dashboard:

```bash
composer config --auth http-basic.repo.example.com YOUR_TOKEN YOUR_PASSWORD
```

See the [Composer client guide](https://packyard.dev/documentation/composer-client) for details.

## Documentation

- [Installation](https://packyard.dev/documentation/installation)
- [Configuration](https://packyard.dev/documentation/configuration)
- [Composer client setup](https://packyard.dev/documentation/composer-client)
- [Organizations](https://packyard.dev/documentation/organizations)

## Development

Prerequisites: [Go 1.25+](https://go.dev/dl/) and [Bun](https://bun.sh).

```bash
git clone https://github.com/usepackyard/packyard.git
cd packyard
make dev
```

Useful commands:

```bash
make test
make build
```

## Contributing

Issues and pull requests are welcome. Please read [CONTRIBUTING.md](CONTRIBUTING.md) before submitting changes.

## License

Packyard is licensed under the [GNU Affero General Public License v3.0](LICENSE).
