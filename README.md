# cloudns

Command-line client for the [ClouDNS HTTP API](https://www.cloudns.net/wiki/article/42/). Install it, log in once, then run short non-interactive commands.

## Install

From a checkout of this repository:

```sh
go install ./cmd/cloudns
```

Or, once the module is published:

```sh
go install github.com/ldrrp/cloudns-cli/cmd/cloudns@latest
```

`go install` puts the binary in `$(go env GOPATH)/bin`. You can also run the built binary from any directory. Publishing a `v*` release, or pushing that tag, attaches Linux, macOS, and Windows archives. Notes written on the release are kept. Each archive contains a `cloudns` binary, and `checksums.txt` lists the SHA-256 hashes.

To copy a binary onto a system bin path:

```sh
cloudns install
```

`cloudns --install` does the same thing. The command copies the current executable to the first writable directory among `/usr/local/bin`, `/usr/bin`, and `~/.local/bin`. Pass `--dir` to choose the directory.

## Log in

ClouDNS has no OAuth login. The auth ID is the numeric API user ID, not the email you use to sign in. Create an API user at [https://www.cloudns.net/api-settings/](https://www.cloudns.net/api-settings/), set a password, and copy the auth ID from the table after you save.

```sh
cloudns auth login
```

The command explains the auth ID, then asks for it and for the API user password. Press Ctrl+C at either prompt to cancel. It calls `GET /login/login.json` and saves credentials only when the response is `{"status":"Success","statusDescription":"Success login."}`. A `Failed` status exits non-zero and writes nothing.

Non-interactive login:

```sh
printf '%s\n' 'your-password' | cloudns auth login --auth-id 12345 --password-stdin
```

Sub-users:

```sh
cloudns auth login --sub-auth-id 555 --password-stdin
cloudns auth login --sub-auth-user alice --password-stdin
```

The auth id (or sub-user id or name) is stored in `~/.config/cloudns/config.json` with mode `0600`. The password is stored in the operating system keyring. If the keyring is unavailable, the password is stored in that same file and a warning is printed once.

```sh
cloudns auth status
cloudns auth logout
```

Logout deletes the keyring item and the config file.

These environment variables override stored credentials for a single invocation and are never written to disk:

- `CLOUDNS_AUTH_ID`
- `CLOUDNS_SUB_AUTH_ID`
- `CLOUDNS_SUB_AUTH_USER`
- `CLOUDNS_AUTH_PASSWORD`

If more than one identity variable is set, `CLOUDNS_SUB_AUTH_USER` wins, then `CLOUDNS_SUB_AUTH_ID`, then `CLOUDNS_AUTH_ID`.

## Workflows

Zones are DNS hosting. Domains are names registered at ClouDNS. A domain's nameservers are the registrar delegation, not the records inside a zone.

```sh
cloudns zone list
cloudns zone list example.com
cloudns zone types
cloudns domain list
cloudns domain list --sort expires
cloudns domain list example.com
```

### Subdomain, DNS stays at ClouDNS

```sh
cloudns zone record add example.com CNAME www my-site.pages.dev --ttl 3600
```

Use `@` as the host for an apex record. `--upsert` updates the existing record of the same type and host instead of failing on a duplicate. `cloudns zone types` lists every record type with an example. `cloudns zone record edit` changes an existing record by id. The type you pass to edit must match the current record; ClouDNS cannot change a record's type.

### Move a domain onto another DNS host

`cloudns domain list` shows the registered domains. Replace one domain's delegation with the two hostnames that DNS host assigned to the zone. They are specific to the zone, not a generic pair.

```sh
cloudns domain nameservers set example.com ada.ns.cloudflare.com bob.ns.cloudflare.com
```

This replaces the full nameserver set. It does not append, and it does not edit NS records inside the zone. The command prints the previous set, then the new set. `cloudns domain list example.com` shows the current delegation.

For `.de`, `.be`, `.ch`, `.fr`, `.re`, `.tf`, `.wf`, `.yt`, `.sh`, and `.eu`, an IPv4 glue address may follow a hostname:

```sh
cloudns domain nameservers set example.be ns1.example.be 203.0.113.10 ns2.example.be 203.0.113.11
```

### Zone files, SOA, and propagation

`cloudns zone info` shows the zone kind. `cloudns zone add` creates one. The default type is master. `cloudns zone delete` removes the zone and its records, not a domain registration, and it requires `--yes`.

```sh
cloudns zone add example.com
cloudns zone info example.com
cloudns zone soa example.com
cloudns zone soa set example.com --admin-mail hostmaster@example.com
cloudns zone export example.com
cloudns zone export --all --dir ./zones
cloudns zone status example.com
cloudns zone delete example.com --yes
```

A slave zone keeps the addresses of the master servers it transfers from:

```sh
cloudns zone add example.com --type slave --master-ip 192.0.2.10
cloudns zone master list example.com
cloudns zone master add example.com 192.0.2.11
cloudns zone master delete example.com 123 --yes
```

## Other commands

```sh
cloudns zone list example.com --type CNAME --host www
cloudns zone record edit example.com 123456 A www 192.0.2.10
cloudns zone record delete example.com 123456 --yes
```

Human-readable tables are the default. `--json` prints the API payload.

Allowed TTL values are 60, 300, 900, 1800, 3600, 21600, 43200, 86400, 172800, 259200, 604800, 1209600, and 2592000. The default is 3600.

`zone delete`, `zone record delete`, and `zone master delete` do not prompt. Pass `--yes` or the command exits without calling the API.

API reference: https://www.cloudns.net/wiki/article/42/
