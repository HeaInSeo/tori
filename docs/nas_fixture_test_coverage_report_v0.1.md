# tori NAS Fixture Test Coverage Report

Date: 2026-06-02
Fixture root: `/mnt/genomics-test/tori-public-fixtures`
Total size: about 20M
Total files: 61
Checksum file: `/mnt/genomics-test/tori-public-fixtures/metadata/SHA256SUMS.txt`

## Scope

This report describes how broadly the current NAS fixture pack can exercise tori tests by file kind, classification axis, and sequencing/analysis stage. The pack is intentionally small and public. It is not a production-scale benchmark corpus.

Genomics fixture files must remain on NAS. Do not copy them into the local checkout or the remote test machine storage.

## 1. File Kind Diversity

| File kind | Count | Representative paths | Test value |
|---|---:|---|---|
| `.fastq.gz` | 20 | `paired_fastq_valid/*.fastq.gz`, `paired_fastq_missing_role/*.fastq.gz`, `paired_fastq_duplicate_role/*.fastq.gz`, `organisms/*/*.fastq.gz` | Raw read discovery, pair-end role resolution, platform-diverse FASTQ naming. |
| `.fasta` | 3 | `reference_annotation/sarscov2_genome.fasta`, `organisms/human_homo_sapiens/human_mt_rCRS.fasta`, `organisms/plant_arabidopsis/ddAraThal4.HiFi.reads.fasta` | Reference/read-set typed views and non-FASTQ sequence inputs. |
| `.fa.gz` | 1 | `organisms/prokaryote_ecoli/genome.fa.gz` | Compressed reference FASTA handling. |
| `.fasta.fai` | 1 | `reference_annotation/sarscov2_genome.fasta.fai` | FASTA primary/index pairing. |
| `.gff3` | 1 | `reference_annotation/sarscov2_genome.gff3` | Annotation typed view. |
| `.gtf` | 1 | `reference_annotation/sarscov2_genome.gtf` | Annotation typed view. |
| `.bam` + `.bam.bai` | 1 + 2 | `alignment_bam/NA12878_chr21_1x.bam*`, `alignment_bam_unpaired_index/NA12878_chr21_1x.bam.bai` | Alignment file/index pairing plus unpaired index negative fixture. |
| `.cram` + `.cram.crai` | 1 + 1 | `alignment_cram/NA12878_chr21_1x.cram*` | CRAM file and index file pairing. |
| `.vcf.gz` + `.vcf.gz.csi` | 2 + 2 | `variant_vcf/*` | Variant file and CSI primary/index pairing. |
| `.bcf` + `.bcf.csi` | 2 + 2 | `variant_bcf/*` | Binary variant file and CSI primary/index pairing. |
| `.csv` | 2 | `paired_fastq_valid/fileblock.csv`, `organisms/mouse_mus_musculus/10x_multiome_lib.csv` | tori output and sequencing/library companion metadata. |
| `.json` | 4 | `paired_fastq_valid/rule.json`, `paired_fastq_invalid/rule.json`, split invalid fixture `rule.json` files | Rule metadata for current FileBlock resolver. |
| `.pb` | 1 | `paired_fastq_valid/paired_fastq_validfiles.pb` | tori generated protobuf output. |
| `.2bit` | 1 | `organisms/human_homo_sapiens/genome.2bit` | Sequence database/index style fixture. |
| `.bed` | 1 | `organisms/human_homo_sapiens/genome.bed` | Interval-format fixture. |
| `.bed.gz` | 1 | `organisms/human_homo_sapiens/genome.bed.gz` | Compressed interval-format fixture. |
| `.gb` | 1 | `organisms/human_homo_sapiens/genome.NC_012920_1.gb` | GenBank reference fixture. |
| `.hmm.gz` | 1 | `organisms/human_homo_sapiens/Pfam-A.hmm.gz` | Profile HMM database fixture. |
| `.dat.gz` | 1 | `organisms/human_homo_sapiens/CTAT_HumanFusionLib.mini.dat.gz` | Fusion reference database fixture. |
| `.tar.gz` | 2 | `organisms/human_homo_sapiens/vep.tar.gz`, `organisms/human_homo_sapiens/vep_cache_113.tar.gz` | Archived reference/tool cache fixtures. |
| `.tsv` | 2 | `organisms/human_homo_sapiens/genome.annotated_intervals.tsv`, `organisms/human_homo_sapiens/arriba.tsv` | Tabular annotation and fusion metadata fixtures. |
| `.zip` | 1 | `organisms/human_homo_sapiens/PRG_test.zip` | Archive/container-format fixture. |
| `.md` / `.txt` | 1 / 3 | `metadata/fixture_manifest.md`, `metadata/SHA256SUMS.txt`, `organisms/human_homo_sapiens/cytoBand_hg38.txt`, `organisms/human_homo_sapiens/fusioncatcher.txt` | Provenance, checksum, and text companion metadata. |

## 2. Classification Axes

### 2.1 Organism diversity

| Organism group | Current coverage | Paths | What this enables |
|---|---|---|---|
| Human | Present | `alignment_*`, `variant_*`, `organisms/human_homo_sapiens/` | Human reference, alignment, and variant typed-view tests. |
| Mouse | Present | `organisms/mouse_mus_musculus/` | Mouse FASTQ and 10x multiome naming/metadata tests. |
| Plant | Present | `organisms/plant_arabidopsis/` | Plant long-read sequence fixture tests. |
| Animal | Present | `organisms/animal_chicken/` | Non-human animal PacBio-style FASTQ tests. |
| Prokaryote | Present | `organisms/prokaryote_ecoli/` | Bacterial reference and paired FASTQ tests. |
| Virus | Present | `paired_fastq_*`, `reference_annotation/` | SARS-CoV-2 pair-end and reference/annotation tests. |

Current organism axis coverage: 6 groups.

### 2.2 Sequencing platform / source style diversity

| Platform/source style | Current coverage | Paths | What this enables |
|---|---|---|---|
| Illumina paired-end short reads | Present | `paired_fastq_valid/`, `paired_fastq_missing_role/`, `paired_fastq_duplicate_role/`, `paired_fastq_invalid/`, `organisms/prokaryote_ecoli/Ecoli_10K_methylated_R*.fastq.gz` | Current pair-end FileBlock grouping, missing role, duplicate role collision. |
| Illumina-style single FASTQ screen reads | Present | `organisms/mouse_mus_musculus/ERR376998.small.fastq.gz`, `ERR376999.small.fastq.gz` | Non-pair-end FASTQ package designs. |
| 10x Genomics multiome | Present | `organisms/mouse_mus_musculus/SRR18907480_chr19_sub_S1_L001_R*.fastq.gz`, `10x_multiome_lib.csv` | Multi-modal naming and companion metadata tests. |
| PacBio / HiFi long reads | Present | `organisms/plant_arabidopsis/ddAraThal4.HiFi.reads.fasta`, `organisms/animal_chicken/pacbio_metagenome.fastq.gz` | Long-read FASTA/FASTQ typed-view tests. |
| Alignment result formats | Present | `alignment_bam/`, `alignment_bam_unpaired_index/`, `alignment_cram/` | Post-alignment package, index pairing, and unpaired index tests. |
| Variant result formats | Present | `variant_vcf/`, `variant_bcf/` | Variant package and index pairing tests. |

Current platform/source-style axis coverage: 6 classes.

### 2.3 Sequencing / analysis stage diversity

| Stage | Current coverage | Paths | What this enables |
|---|---|---|---|
| Raw sequencing reads | Present | `*.fastq.gz` under `paired_fastq_*` and `organisms/*` | Read package discovery and role grouping. |
| Long-read read sets | Present | Arabidopsis HiFi FASTA, chicken PacBio FASTQ | Non-Illumina sequence input handling. |
| Reference genome | Present | `reference_annotation/*.fasta`, `organisms/*/*.fasta`, `organisms/prokaryote_ecoli/genome.fa.gz` | Reference typed-view tests. |
| Reference index | Present | `reference_annotation/sarscov2_genome.fasta.fai` | Reference primary/index pairing. |
| Annotation | Present | `reference_annotation/*.gff3`, `*.gtf` | Annotation typed-view tests. |
| Alignment output | Present | `alignment_bam/*.bam`, `alignment_cram/*.cram` | Alignment package tests. |
| Alignment index | Present | `*.bai`, `*.crai` | Alignment primary/index pairing. |
| Variant output | Present | `variant_vcf/*.vcf.gz`, `variant_bcf/*.bcf` | Variant package tests. |
| Variant index | Present | `*.csi` | Variant primary/index pairing. |
| Library/sample metadata | Present | `organisms/mouse_mus_musculus/10x_multiome_lib.csv` | Companion metadata tests. |
| tori generated result | Present | `paired_fastq_valid/fileblock.csv`, `paired_fastq_valid/paired_fastq_validfiles.pb` | Regression check for actual tori output generated from NAS input. |

Current sequencing/analysis stage coverage: 11 stages.

## 3. Current Testability Summary

| Test theme | Current status | Notes |
|---|---|---|
| Current Track A pair-end happy path | Ready now | `paired_fastq_valid/` has rule.json and passed actual tori FileBlock generation. |
| Duplicate role collision | Ready now | `paired_fastq_duplicate_role/` surfaces `DuplicateCollisionError` independently. |
| Missing role handling | Ready now | `paired_fastq_missing_role/` exercises missing R2 without being masked by duplicate collision. |
| Multi-organism typed-view design | Fixture ready, code/spec pending | Organism directories provide human/mouse/plant/animal/prokaryote/virus coverage. |
| Multi-platform typed-view design | Fixture ready, code/spec pending | Illumina, 10x, PacBio/HiFi, alignment, and variant formats are represented. |
| Analysis-stage package design | Fixture ready, code/spec pending | Raw reads, references, annotation, alignment, variant, indexes, metadata, and archive/container-like companion files are represented. |
| Primary/index pairing | Fixture ready, BAM/BAI and CRAM/CRAI smoke connected | BAM/BAI complete, BAI-only unpaired index, and CRAM/CRAI complete fixtures are covered; VCF/CSI, BCF/CSI, FASTA/FAI remain available for later design. |
| Output regression from real NAS input | Ready now | Existing `fileblock.csv` and `.pb` outputs were generated from NAS valid pair-end input. |

## 4. Directory Size Distribution

| Directory | Size | Interpretation |
|---|---:|---|
| `organisms/` | 16M | Main diversity corpus across organism and platform axes. |
| `variant_bcf/` | 2.0M | Binary variant fixtures and CSI index files. |
| `variant_vcf/` | 782K | Compressed VCF fixtures and CSI index files. |
| `alignment_bam/` | 275K | BAM + BAI fixture. |
| `alignment_bam_unpaired_index/` | 36K | BAI-only unpaired index negative fixture. |
| `alignment_cram/` | 130K | CRAM + CRAI fixture. |
| `paired_fastq_valid/` | 74K | Current resolver happy-path fixture plus generated tori outputs. |
| `paired_fastq_invalid/` | 67K | Legacy mixed invalid fixture containing both missing-role and duplicate-role cases. |
| `paired_fastq_missing_role/` | about 10K | Split missing R2 fixture for independent invalid-row observation. |
| `paired_fastq_duplicate_role/` | about 29K | Split duplicate R1 fixture for independent duplicate collision observation. |
| `reference_annotation/` | 50K | Reference and annotation fixtures. |
| `metadata/` | 19K | Manifest and checksum metadata. |

## 5. Recommended Next Fixture Splits

1. Add explicit `rule.json` files for future typed-view classes:
   - reference + annotation package
   - alignment + index package
   - variant + index package
   - 10x multiome package
2. Add a small ONT-specific fixture if public small data is selected later. The current pack has PacBio/HiFi long-read coverage, but no explicit ONT-labeled fixture yet.
3. Continue using the shared filesystem smoke command in the repo so actual tori tests consistently target this path.

## 6. Bottom Line

The current NAS corpus is broad enough for immediate Track A pair-end regression tests and for designing future multi-role typed views across at least:

- 28 file-kind classes
- 6 organism groups
- 6 sequencing/platform source-style classes
- 11 sequencing/analysis stages

The remaining limitation is not fixture availability, but typed-view rule/spec coverage in tori. Current code can directly exercise the pair-end FileBlock path; the other fixture groups are prepared as design and regression anchors for the next Track A increments.
