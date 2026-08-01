# Common Types

The following common types are reused across multiple configuration items. Specific configuration items reference these types by name in their validity conditions.

## 1. Port

- Type: Integer.
- Valid range: 1-65535 (IANA valid port range, RFC 793).

## 2. ListenAddr

- Type: String.
- Empty string means listen on all addresses (i.e., `0.0.0.0` / `::`).
- If non-empty, must be a valid IPv4/IPv6 address or an RFC 1123 hostname.

## 3. FilePath

- Type: String.
- If not configured, falls back to the corresponding default value.
- Supports relative paths (resolved relative to the BFE configuration root directory) or absolute paths (starting with `/`).
- The referenced file must exist and be readable at runtime.

## 4. DirPath

- Type: String.
- If not configured, falls back to the corresponding default value.
- Supports relative paths (resolved relative to the BFE configuration root directory) or absolute paths (starting with `/`).
- The referenced directory must exist and be readable at runtime.

## 5. Version

- Type: String.
- Usually a timestamp string, e.g., `20190101000000`.
- Identifies the version of the configuration file.

## 6. Hostname

- Type: String.
- Must be an RFC 1123 hostname or a valid IPv4/IPv6 address.
- Total hostname length must not exceed 255 characters.
- Each label (separated by `.`) must not exceed 63 characters.
- Labels may only contain letters, digits, and hyphens (`-`).
- Labels must not start or end with a hyphen (`-`).
- **Length must be ≥ 2 characters**.

## 7. IPAddr

- Type: String.
- Must be an IPv4 address per RFC 791 or an IPv6 address per RFC 8200.
- **IPv4**: dotted-decimal, 4 octets, each 0-255, e.g., `192.0.2.1`.
- **IPv6**: 8 groups of 16-bit hex, groups separated by `:`, supports `::` compression, e.g., `2001:0db8::1`.

## 8. Weight

- Type: Integer.
- Non-negative integer representing a weight proportion.
- Specific constraints depend on the semantics of the configuration item (e.g., whether 0 is allowed, whether the sum must be greater than 0, etc.).

## 9. HTTPStatusCodePattern

- Type: String.
- Supports specific status codes (e.g., `"500"`) or status code ranges (e.g., `"4xx"`, `"5xx"`).
- Multiple patterns may be joined with `|`, e.g., `"503|4xx"`.
