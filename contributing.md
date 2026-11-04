# Contributing

Contributions are welcome, including bug fixes, improvements, documentation, and new features for the OpenRGB API service.

## Getting Started

You need Go 1.26.1 or newer to build and test this project. To exercise the service against real hardware, you also need OpenRGB running with its SDK server enabled.

Clone the repository:

```sh
git clone https://github.com/sb0-0dev/openrgb-api.git
cd openrgb-api
```

Run the tests (this also downloads dependencies as needed):

```sh
go test ./...
```

To run the service locally:

```sh
go run ./cmd/openrgb-api
```

For configuration, ports, and building installers, see [README.md](README.md) and [documentation/setup.md](documentation/setup.md) (especially Building from source).

## Making Changes

Create a branch for your changes:

```sh
git checkout -b fix/my-change
```

Keep changes focused and follow the existing Go style and conventions. Format Go code before submitting:

```sh
gofmt -w ./cmd
```

You can also check Go code with:

```sh
go vet ./...
```

If you change behavior, add or update the relevant tests. The HTTP handlers, authentication, pairing, server behavior, and configuration have tests under `cmd/openrgb-api`.

## API and Documentation Changes

When changing an endpoint, keep the handler behavior, response models, tests, API reference, and OpenAPI specification in sync. Update `documentation/api_reference.md` and `documentation/openapi.yaml` when requests, responses, errors, or authentication requirements change. The API reference is also served through the control panel's `/docs` page; its embedded documentation is maintained in `cmd/openrgb-api/docs.go` and `documentation/embed.go`.

For changes to the control panel, update the relevant files under `cmd/openrgb-api/web`. For configuration, installation, or runtime changes, update `README.md` and/or `documentation/setup.md` as appropriate. Public Go types and functions should have clear GoDoc comments.

## Testing

Run the full test suite before opening a pull request:

```sh
go test ./...
```

For detailed test output:

```sh
go test -v ./...
```

If your change affects interaction with OpenRGB, pairing, network listeners, or packaging, also describe any manual verification that requires a running OpenRGB server or a particular operating system. Do not expose the service to an untrusted network while testing; see the security guidance in [README.md](README.md).

## Pull Requests

When opening a pull request, please:

- Explain what changed and why.
- Include tests for new or changed behavior.
- Run the checks listed under [Making Changes](#making-changes) and [Testing](#testing).
- Update the API reference, OpenAPI specification, or setup documentation when relevant.
- Keep the pull request focused on one change where possible.

## AI-Assisted Contributions

AI coding tools are permitted, but contributors remain responsible for all code they submit. Review AI-generated code for correctness, security, maintainability, licensing concerns, and consistency with this project. AI use does not replace the contributor's responsibility for understanding and maintaining their contribution.

## Commit Messages

Use clear, descriptive commit messages. For example:

```text
Add support for custom timeouts
Update API authentication documentation
```

## Reporting Bugs

Please open an issue and include:

- A description of the problem and steps to reproduce it.
- The expected and actual behavior.
- Your operating system and Go version, when relevant.
- Relevant API requests, responses, logs, or error messages. Remove bearer tokens, pairing codes, and other secrets before sharing.
- Whether the issue requires a running OpenRGB SDK server or particular hardware.

## Questions

If you're unsure about a contribution, feel free to open an issue or discussion.

Thanks for contributing!
