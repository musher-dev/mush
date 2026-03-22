# Error Runbook

Stable error IDs for common remediation paths.

## `ERR-AUTH-001` Authentication Failed

- Symptom: `mush auth login` / `mush auth status` fails.
- Checks:
  - Validate `MUSHER_API_KEY` value.
  - Re-run `mush auth login`.
  - Run `mush doctor` for connectivity and clock-skew checks.

## `ERR-NET-001` TLS Certificate Trust Failure

- Symptom: x509/certificate/TLS verification failures.
- Cause: often corporate proxy interception.
- Fix:
  - Configure `MUSHER_NETWORK_CA_CERT_FILE=/path/to/ca-bundle.pem`.
  - Re-run `mush doctor` and `mush auth status`.

## `ERR-NET-002` Clock Skew

- Symptom: token appears expired/not-yet-valid unexpectedly.
- Fix:
  - Sync machine clock with NTP.
  - Re-run `mush doctor` and retry auth.

## `ERR-QUEUE-001` No Active Queue Instruction

- Symptom: worker start fails with no active instruction.
- Fix:
  - Activate an instruction for the queue in the platform console.
  - Re-run `mush worker start --dry-run`.

## Linking Convention

- CLI hints link to the canonical runbook at:
  - `https://github.com/musher-dev/mush/blob/main/docs/errors.md#err-auth-001-authentication-failed`
  - `https://github.com/musher-dev/mush/blob/main/docs/errors.md#err-net-001-tls-certificate-trust-failure`
  - `https://github.com/musher-dev/mush/blob/main/docs/errors.md#err-net-002-clock-skew`
  - `https://github.com/musher-dev/mush/blob/main/docs/errors.md#err-queue-001-no-active-queue-instruction`
