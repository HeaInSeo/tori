# Track A NAS Completion and Remaining Work
### Date: 2026-06-12
### Status: NAS baseline closed; Lustre deferred

## 1. Scope

This note closes the current NAS-based Track A shared filesystem fixture work.

The active fixture root is:

- `/mnt/genomics-test/tori-public-fixtures`

Lustre is not part of the current execution scope because the Lustre fixture mount is not ready.
When the mount is ready, the same smoke contract should be re-run with `TORI_SHARED_FIXTURE_ROOT` pointing at the Lustre fixture root.

## 2. Completed NAS Smoke Coverage

The following NAS fixture probes are connected to `make test-shared-fs-fixtures`.

Pair-end regression:

- `paired_fastq_valid/`
- `paired_fastq_missing_role/`
- `paired_fastq_duplicate_role/`

Alignment primary/index:

- `alignment_bam/`
- `alignment_bam_unpaired_index/`
- `alignment_cram/`

Variant primary/index:

- `variant_vcf/`
- `variant_bcf/`

Reference primary/index:

- `reference_annotation/sarscov2_genome.fasta`
- `reference_annotation/sarscov2_genome.fasta.fai`

## 3. Current Verification Commands

NAS fixture gate:

```sh
make test-shared-fs-fixtures
```

Core CI-equivalent local gate:

```sh
make test-core
make lint
```

GitHub Actions gate:

- `core-ci`

### 3.1 Sprint 0 re-verification record (2026-06-21)

The NAS baseline was re-verified from the current checkout with the local stale
`GOROOT` setting removed for the command invocation.

```sh
env -u GOROOT make doctor
env -u GOROOT make lint
env -u GOROOT make test-core
env -u GOROOT make test-shared-fs-fixtures
cd /mnt/genomics-test/tori-public-fixtures
sha256sum -c metadata/SHA256SUMS.txt
```

Results:

- `make doctor`, `make lint`, `make test-core`, and the NAS shared-fixture smoke gate passed.
- Every artifact listed in `metadata/SHA256SUMS.txt` passed checksum verification.
- `SHA256SUMS.txt` itself is not self-verified by its own manifest; its storage/provenance remains an external fixture-management concern.
- Lustre parity remains deferred until a Lustre fixture mount is available.

## 4. Remaining Work

Immediate NAS Track A primary/index fixture work:

- None.

Deferred work:

- Re-run `make test-shared-fs-fixtures` against Lustre after the Lustre fixture mount is prepared.
- Decide whether to add negative unpaired-index fixtures for CRAM/CRAI, VCF/CSI, BCF/CSI, and FASTA/FAI.
- Keep public diagnostics/API changes out of the current Track A fixture baseline unless explicitly scoped.
- Keep service/gRPC/K8s/runtime recovery outside this Track A fixture closure.

## 5. Cleanup Policy

The following are safe to remove from development machines after verification:

- repo-local Go build cache such as `.go-cache/`
- temporary patch files under `/tmp/tori-*.patch`
- temporary verification clones under `/tmp/tori-codex-*`

The following are not cleanup targets:

- tracked `reports/` files
- NAS fixture files under `/mnt/genomics-test/tori-public-fixtures`
- generated fixture manifest and checksum files in the NAS fixture metadata directory
