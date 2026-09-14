# Changelog

All notable changes to this module are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this module adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-09-12

First release: JSON HTTP APIs on Echo v5 with the building blocks of httpx.

- `Make` and `Config` for a conventional application: the service's
  `*slog.Logger`, the validator, the RFC 9457 error handler, logging of 5xx
  responses, Echo's `Recover`, an optional `BodyLimit` (none by default, as in
  Echo), application middleware and `GET /ping`.
- Typed handler chains: `MakeHandler` with `PositivePathID`, `Query`, `Headers`,
  `JSON` and `Bind` steps for one or two inputs, and `Data`, `Created` and
  `NoContent` terminals that call a service method checked by the compiler.
- Binding by source with Echo's binder: `BindPathAndValidate`,
  `BindQueryAndValidate`, `BindHeadersAndValidate`, `BindPositivePathID`, and
  strict JSON bodies with `DecodeJSON` and `DecodeAndValidateJSON`; an
  undecodable value is a `BindError` with code `type` and the path of its
  field.
- `MakeValidator`: go-playground/validator with its English translations, field
  names from `json`, `query`, `param` and `header` tags, the `enum` and
  `notblank` tags, and `GoPlayground` and `Translator` for application tags;
  failures are a `ValidationError` with tag names as codes.
- `Requests` with `BindPageQuery` and `BindListQuery`: endpoint filters, page
  and size under a validated `page.Config`, and repeated `sort` parameters
  parsed into a typed order through `EnumStringable`.
- Responses through Echo's JSON serializer: `Write`, `OK`, `Data`, `Created`
  and `Page`.
- `MakeErrorHandler`, `MakeProblemMapper` and `BindProblemRule`: every error
  as Problem Details; Echo's own 404, 405 and 413 keep their status, unknown
  errors never leak, committed responses are not replaced, and HEAD gets
  headers without a body.

[0.1.0]: https://github.com/uchaloop/echox/releases/tag/v0.1.0
