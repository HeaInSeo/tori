# tori NAS Fixture Run Results

Date: 2026-06-02
Fixture root: `/mnt/genomics-test/tori-public-fixtures`
Execution note: genomics fixtures are stored on NAS only. The temporary Go runner used for this check was not a genomics data file.

## Valid paired FASTQ fixture

Command shape:

```sh
go run /tmp/tori_run_fileblock.go /mnt/genomics-test/tori-public-fixtures/paired_fastq_valid
```

Observed result:

- Exit status: 0
- Headers: `[R1 R2]`
- Rows: 2
- Row 0: `nfcoreA_S1_L001_001_fastq_gz` with R1/R2 files
- Row 1: `nfcoreB_S2_L001_001_fastq_gz` with R1/R2 files
- Generated files:
  - `paired_fastq_valid/fileblock.csv`
  - `paired_fastq_valid/paired_fastq_validfiles.pb`

## Invalid paired FASTQ fixture

Command shape:

```sh
go run /tmp/tori_run_fileblock.go /mnt/genomics-test/tori-public-fixtures/paired_fastq_invalid
```

Observed result:

- Exit status: 3
- Error type: `*errs.DuplicateCollisionError`
- Reason: `duplicate_role_in_row`
- RowKey: `nfcoreD_S4_L001_001_fastq_gz`
- RoleKey: `R1`
- Candidate files:
  - `nfcoreD_S4_L001_R1_001.fastq.gz`
  - `nfcoreD__S4_L001_R1_001.fastq.gz`

## Interpretation

- The valid pair-end fixture exercises the current Track A FileBlock resolver happy path.
- The invalid fixture confirms that duplicate role collision is surfaced as an explicit error instead of being silently resolved.
- Missing-role behavior remains available in the same invalid directory, but the duplicate collision currently stops the run first.

## Split invalid paired FASTQ fixtures

Sprint 1 split the mixed invalid fixture into independent shared filesystem fixture directories:

- `paired_fastq_missing_role/`
- `paired_fastq_duplicate_role/`

Current smoke command shape:

```sh
make test-shared-fs-fixtures
```

Observed result:

- `paired_fastq_missing_role/` exits through the invalid-row path without duplicate collision.
- `paired_fastq_duplicate_role/` surfaces `*rules.DuplicateCollisionError`.
- Generated `fileblock.csv`, `.pb`, and `invalid_files_*` outputs are written only to test temporary directories, not to the shared filesystem fixture directory.

## Pair-end regression contract

Sprint 2 promoted the split pair-end fixtures into a Track A regression contract.

Contract document:

- `docs/track_a_pair_end_regression_contract_v0.1.md`

Current coverage:

- `paired_fastq_valid/`: headers `R1/R2`, 2 valid rows, generated CSV and protobuf output.
- `paired_fastq_missing_role/`: 0 valid rows and one invalid files report, without duplicate collision.
- `paired_fastq_duplicate_role/`: typed duplicate collision error with `duplicate_role_in_row`.
