package block

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/seoyhaein/tori/rules"
)

func TestSharedFSFixtureSmoke_PairedFASTQ(t *testing.T) {
	root := os.Getenv("TORI_SHARED_FIXTURE_ROOT")
	if root == "" {
		root = os.Getenv("TORI_NAS_FIXTURE_ROOT")
	}
	if root == "" {
		t.Skip("TORI_SHARED_FIXTURE_ROOT is not set")
	}

	validDir := filepath.Join(root, "paired_fastq_valid")
	validWorkDir, validFiles := prepareSharedFSFixtureWorkDir(t, validDir)
	validPreview, err := rules.GenerateResolverPreviewFromDir(validWorkDir)
	if err != nil {
		t.Fatalf("GenerateResolverPreviewFromDir valid paired FASTQ fixture error: %v", err)
	}
	if validPreview.SourceFileCount != 4 || validPreview.RowCount != 2 || validPreview.ObservedRoleCount != 4 {
		t.Fatalf("unexpected valid paired FASTQ preview summary: %#v", validPreview)
	}
	if validPreview.UnresolvedRoleCount != 4 {
		t.Fatalf("expected R1/R2 to remain unresolved without roleNormalization, got %#v", validPreview)
	}
	for _, row := range validPreview.Rows {
		if len(row.ObservedRoles) != 2 || row.ObservedRoles[0] != "R1" || row.ObservedRoles[1] != "R2" {
			t.Fatalf("unexpected valid paired FASTQ observed roles: %#v", row)
		}
	}
	validRuleSet := loadSharedFSFixtureRuleSet(t, validWorkDir)
	validSchemaPreview := rules.BuildSchemaValidationPreview(validPreview, validRuleSet)
	if validSchemaPreview.MissingRequiredRoleCount != 0 || validSchemaPreview.ExtraObservedRoleCount != 0 {
		t.Fatalf("expected valid paired FASTQ schema preview to have no missing/extra candidates, got %#v", validSchemaPreview)
	}
	if validSchemaPreview.UnresolvedObservedRoleCount != 4 || validSchemaPreview.EntryCount != 4 {
		t.Fatalf("expected valid paired FASTQ schema preview to keep R1/R2 unresolved observations, got %#v", validSchemaPreview)
	}
	assertNoGeneratedPreviewOutputs(t, validWorkDir)

	fb, err := GenerateFileBlock(validWorkDir, validFiles)
	if err != nil {
		t.Fatalf("GenerateFileBlock valid fixture error: %v", err)
	}
	if len(fb.GetColumnHeaders()) != 2 || fb.GetColumnHeaders()[0] != "R1" || fb.GetColumnHeaders()[1] != "R2" {
		t.Fatalf("unexpected headers: %#v", fb.GetColumnHeaders())
	}
	if len(fb.GetRows()) != 2 {
		t.Fatalf("expected 2 valid paired FASTQ rows, got %d", len(fb.GetRows()))
	}
	assertGeneratedFileBlockOutputs(t, validWorkDir)

	missingDir := filepath.Join(root, "paired_fastq_missing_role")
	missingWorkDir, missingFiles := prepareSharedFSFixtureWorkDir(t, missingDir)
	missingPreview, err := rules.GenerateResolverPreviewFromDir(missingWorkDir)
	if err != nil {
		t.Fatalf("GenerateResolverPreviewFromDir missing-role paired FASTQ fixture error: %v", err)
	}
	if missingPreview.SourceFileCount != 1 || missingPreview.RowCount != 1 || missingPreview.ObservedRoleCount != 1 {
		t.Fatalf("unexpected missing-role paired FASTQ preview summary: %#v", missingPreview)
	}
	if missingPreview.UnresolvedRoleCount != 1 {
		t.Fatalf("expected missing-role R1 to remain unresolved without roleNormalization, got %#v", missingPreview)
	}
	if len(missingPreview.Rows) != 1 || len(missingPreview.Rows[0].ObservedRoles) != 1 || missingPreview.Rows[0].ObservedRoles[0] != "R1" {
		t.Fatalf("unexpected missing-role paired FASTQ observed roles: %#v", missingPreview.Rows)
	}
	missingRuleSet := loadSharedFSFixtureRuleSet(t, missingWorkDir)
	missingSchemaPreview := rules.BuildSchemaValidationPreview(missingPreview, missingRuleSet)
	if missingSchemaPreview.UnresolvedObservedRoleCount != 1 {
		t.Fatalf("expected missing-role schema preview to keep R1 unresolved observation, got %#v", missingSchemaPreview)
	}
	if missingSchemaPreview.MissingRequiredRoleCount != 1 || missingSchemaPreview.ExtraObservedRoleCount != 0 {
		t.Fatalf("expected missing-role schema preview to have one missing required candidate only, got %#v", missingSchemaPreview)
	}
	if missingSchemaPreview.EntryCount != 2 {
		t.Fatalf("expected missing-role schema preview to keep unresolved and missing entries separate, got %#v", missingSchemaPreview)
	}
	if missingSchemaPreview.Entries[1].ReasonCode != "missing_required_role" || missingSchemaPreview.Entries[1].Role != "R2" {
		t.Fatalf("expected missing-role schema preview to report missing R2, got %#v", missingSchemaPreview.Entries)
	}
	assertNoGeneratedPreviewOutputs(t, missingWorkDir)

	missingFB, err := GenerateFileBlock(missingWorkDir, missingFiles)
	if err != nil {
		t.Fatalf("GenerateFileBlock missing-role fixture error: %v", err)
	}
	if len(missingFB.GetRows()) != 0 {
		t.Fatalf("expected missing-role fixture to produce 0 valid rows, got %d", len(missingFB.GetRows()))
	}
	matches, err := filepath.Glob(filepath.Join(missingWorkDir, "invalid_files_*.txt"))
	if err != nil {
		t.Fatalf("glob missing-role invalid report: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one missing-role invalid report, got %d", len(matches))
	}
	assertGeneratedFileBlockOutputs(t, missingWorkDir)

	duplicateDir := filepath.Join(root, "paired_fastq_duplicate_role")
	duplicateWorkDir, duplicateFiles := prepareSharedFSFixtureWorkDir(t, duplicateDir)
	_, err = rules.GenerateResolverPreviewFromDir(duplicateWorkDir)
	if err == nil {
		t.Fatalf("expected duplicate collision error from duplicate-role preview")
	}

	var previewDupErr *rules.DuplicateCollisionError
	if !errors.As(err, &previewDupErr) {
		t.Fatalf("expected preview DuplicateCollisionError, got %T: %v", err, err)
	}
	if len(previewDupErr.Entries) == 0 {
		t.Fatalf("expected preview duplicate entries")
	}
	previewEntry := previewDupErr.Entries[0]
	if previewEntry.ReasonCode != "duplicate_role_in_row" || previewEntry.RoleKey != "R1" {
		t.Fatalf("unexpected preview duplicate entry: %#v", previewEntry)
	}
	assertNoGeneratedPreviewOutputs(t, duplicateWorkDir)

	_, err = GenerateFileBlock(duplicateWorkDir, duplicateFiles)
	if err == nil {
		t.Fatalf("expected duplicate collision error for duplicate-role fixture")
	}

	var dupErr *rules.DuplicateCollisionError
	if !errors.As(err, &dupErr) {
		t.Fatalf("expected DuplicateCollisionError, got %T: %v", err, err)
	}
	if len(dupErr.Entries) == 0 {
		t.Fatalf("expected duplicate entries")
	}
	entry := dupErr.Entries[0]
	if entry.ReasonCode != "duplicate_role_in_row" || entry.RoleKey != "R1" {
		t.Fatalf("unexpected duplicate entry: %#v", entry)
	}
}

func TestSharedFSFixtureSmoke_AlignmentBAMIndexFixtureSpecificCurrentRuleProbe(t *testing.T) {
	root := os.Getenv("TORI_SHARED_FIXTURE_ROOT")
	if root == "" {
		root = os.Getenv("TORI_NAS_FIXTURE_ROOT")
	}
	if root == "" {
		t.Skip("TORI_SHARED_FIXTURE_ROOT is not set")
	}

	alignmentDir := filepath.Join(root, "alignment_bam")
	ruleSet := rules.RuleSet{
		Version:     "1",
		Delimiter:   []string{"_", "."},
		Header:      []string{"bam", "bam_bai"},
		RowRules:    rules.RowRules{MatchParts: []int{0, 1, 2}},
		ColumnRules: rules.ColumnRules{MatchParts: []int{3, 4}},
		SizeRules:   rules.SizeRules{MinSize: 0, MaxSize: 104857600},
		RoleNormalization: map[string]string{
			"bam":     "BAM",
			"bam_bai": "BAI",
		},
	}
	workDir, fileNames := prepareSharedFSFixtureWorkDirWithRule(t, alignmentDir, ruleSet)

	preview, err := rules.GenerateResolverPreviewFromDir(workDir)
	if err != nil {
		t.Fatalf("GenerateResolverPreviewFromDir alignment BAM/BAI probe error: %v", err)
	}
	if preview.RowCount != 1 || len(preview.Rows) != 1 {
		t.Fatalf("expected 1 alignment preview row, got %#v", preview)
	}
	if preview.SourceFileCount != 2 || preview.ObservedRoleCount != 2 || preview.UnresolvedRoleCount != 0 {
		t.Fatalf("unexpected alignment preview summary: %#v", preview)
	}
	if preview.Rows[0].RoleNormalization[0].NormalizedRole != "BAM" || preview.Rows[0].RoleNormalization[1].NormalizedRole != "BAI" {
		t.Fatalf("unexpected alignment normalization preview: %#v", preview.Rows[0].RoleNormalization)
	}
	schemaPreview := rules.BuildSchemaValidationPreview(preview, ruleSet)
	if schemaPreview.MissingRequiredRoleCount != 0 || schemaPreview.ExtraObservedRoleCount != 0 || schemaPreview.UnresolvedObservedRoleCount != 0 {
		t.Fatalf("expected alignment fixture-specific schema preview to have no candidates, got %#v", schemaPreview)
	}
	pairingPreview := rules.BuildPrimaryIndexPairingPreview(preview, "BAM", "BAI")
	if pairingPreview.EntryCount != 0 {
		t.Fatalf("expected complete alignment fixture primary/index pairing preview to have no candidates, got %#v", pairingPreview)
	}
	assertNoGeneratedPreviewOutputs(t, workDir)

	unpairedAlignmentDir := filepath.Join(root, "alignment_bam_unpaired_index")
	unpairedIndexWorkDir, _ := prepareSharedFSFixtureWorkDirWithRule(t, unpairedAlignmentDir, ruleSet)
	unpairedIndexPreview, err := rules.GenerateResolverPreviewFromDir(unpairedIndexWorkDir)
	if err != nil {
		t.Fatalf("GenerateResolverPreviewFromDir unpaired BAI index probe error: %v", err)
	}
	if unpairedIndexPreview.SourceFileCount != 1 || unpairedIndexPreview.RowCount != 1 || unpairedIndexPreview.ObservedRoleCount != 1 || unpairedIndexPreview.UnresolvedRoleCount != 0 {
		t.Fatalf("unexpected unpaired BAI index preview summary: %#v", unpairedIndexPreview)
	}
	if unpairedIndexPreview.Rows[0].RoleNormalization[0].ObservedKey != "bam_bai" || unpairedIndexPreview.Rows[0].RoleNormalization[0].NormalizedRole != "BAI" {
		t.Fatalf("unexpected unpaired BAI index normalization preview: %#v", unpairedIndexPreview.Rows[0].RoleNormalization)
	}
	unpairedIndexSchemaPreview := rules.BuildSchemaValidationPreview(unpairedIndexPreview, ruleSet)
	if unpairedIndexSchemaPreview.MissingRequiredRoleCount != 1 || unpairedIndexSchemaPreview.Entries[0].Role != "bam" {
		t.Fatalf("expected unpaired BAI index observed-key schema preview to report missing bam, got %#v", unpairedIndexSchemaPreview)
	}
	unpairedIndexTypedPreview := rules.BuildTypedRoleValidationPreview(unpairedIndexPreview, []string{"BAM", "BAI"})
	if unpairedIndexTypedPreview.MissingRequiredRoleCount != 1 || unpairedIndexTypedPreview.Entries[0].Role != "BAM" {
		t.Fatalf("expected unpaired BAI index typed-role preview to report missing BAM, got %#v", unpairedIndexTypedPreview)
	}
	unpairedIndexPairingPreview := rules.BuildPrimaryIndexPairingPreview(unpairedIndexPreview, "BAM", "BAI")
	if unpairedIndexPairingPreview.EntryCount != 1 || unpairedIndexPairingPreview.Entries[0].ReasonCode != "unpaired_index_role" {
		t.Fatalf("expected unpaired BAI index pairing preview candidate, got %#v", unpairedIndexPairingPreview)
	}
	assertNoGeneratedPreviewOutputs(t, unpairedIndexWorkDir)

	fb, err := GenerateFileBlock(workDir, fileNames)
	if err != nil {
		t.Fatalf("GenerateFileBlock alignment BAM/BAI probe error: %v", err)
	}
	if len(fb.GetColumnHeaders()) != 2 || fb.GetColumnHeaders()[0] != "bam" || fb.GetColumnHeaders()[1] != "bam_bai" {
		t.Fatalf("unexpected alignment probe headers: %#v", fb.GetColumnHeaders())
	}
	if len(fb.GetRows()) != 1 {
		t.Fatalf("expected 1 alignment BAM/BAI row, got %d", len(fb.GetRows()))
	}
	row := fb.GetRows()[0]
	if row.GetCells()["bam"] != "NA12878_chr21_1x.bam" {
		t.Fatalf("unexpected BAM cell: %#v", row.GetCells())
	}
	if row.GetCells()["bam_bai"] != "NA12878_chr21_1x.bam.bai" {
		t.Fatalf("unexpected BAI cell: %#v", row.GetCells())
	}
	if _, ok := row.GetCells()["BAM"]; ok {
		t.Fatalf("expected fixture probe cells to keep observed keys, got normalized BAM key in %#v", row.GetCells())
	}
	if _, ok := row.GetCells()["BAI"]; ok {
		t.Fatalf("expected fixture probe cells to keep observed keys, got normalized BAI key in %#v", row.GetCells())
	}
	assertGeneratedFileBlockOutputs(t, workDir)
}

func TestSharedFSFixtureSmoke_AlignmentCRAMIndexFixtureSpecificCurrentRuleProbe(t *testing.T) {
	root := os.Getenv("TORI_SHARED_FIXTURE_ROOT")
	if root == "" {
		root = os.Getenv("TORI_NAS_FIXTURE_ROOT")
	}
	if root == "" {
		t.Skip("TORI_SHARED_FIXTURE_ROOT is not set")
	}

	alignmentDir := filepath.Join(root, "alignment_cram")
	ruleSet := rules.RuleSet{
		Version:     "1",
		Delimiter:   []string{"_", "."},
		Header:      []string{"cram", "cram_crai"},
		RowRules:    rules.RowRules{MatchParts: []int{0, 1, 2}},
		ColumnRules: rules.ColumnRules{MatchParts: []int{3, 4}},
		SizeRules:   rules.SizeRules{MinSize: 0, MaxSize: 104857600},
		RoleNormalization: map[string]string{
			"cram":      "CRAM",
			"cram_crai": "CRAI",
		},
	}
	workDir, fileNames := prepareSharedFSFixtureWorkDirWithRule(t, alignmentDir, ruleSet)

	preview, err := rules.GenerateResolverPreviewFromDir(workDir)
	if err != nil {
		t.Fatalf("GenerateResolverPreviewFromDir alignment CRAM/CRAI probe error: %v", err)
	}
	if preview.RowCount != 1 || len(preview.Rows) != 1 {
		t.Fatalf("expected 1 CRAM alignment preview row, got %#v", preview)
	}
	if preview.SourceFileCount != 2 || preview.ObservedRoleCount != 2 || preview.UnresolvedRoleCount != 0 {
		t.Fatalf("unexpected CRAM alignment preview summary: %#v", preview)
	}
	if preview.Rows[0].RoleNormalization[0].NormalizedRole != "CRAM" || preview.Rows[0].RoleNormalization[1].NormalizedRole != "CRAI" {
		t.Fatalf("unexpected CRAM alignment normalization preview: %#v", preview.Rows[0].RoleNormalization)
	}
	schemaPreview := rules.BuildSchemaValidationPreview(preview, ruleSet)
	if schemaPreview.MissingRequiredRoleCount != 0 || schemaPreview.ExtraObservedRoleCount != 0 || schemaPreview.UnresolvedObservedRoleCount != 0 {
		t.Fatalf("expected CRAM fixture-specific schema preview to have no candidates, got %#v", schemaPreview)
	}
	pairingPreview := rules.BuildPrimaryIndexPairingPreview(preview, "CRAM", "CRAI")
	if pairingPreview.EntryCount != 0 {
		t.Fatalf("expected complete CRAM fixture primary/index pairing preview to have no candidates, got %#v", pairingPreview)
	}
	assertNoGeneratedPreviewOutputs(t, workDir)

	fb, err := GenerateFileBlock(workDir, fileNames)
	if err != nil {
		t.Fatalf("GenerateFileBlock alignment CRAM/CRAI probe error: %v", err)
	}
	if len(fb.GetColumnHeaders()) != 2 || fb.GetColumnHeaders()[0] != "cram" || fb.GetColumnHeaders()[1] != "cram_crai" {
		t.Fatalf("unexpected CRAM alignment probe headers: %#v", fb.GetColumnHeaders())
	}
	if len(fb.GetRows()) != 1 {
		t.Fatalf("expected 1 alignment CRAM/CRAI row, got %d", len(fb.GetRows()))
	}
	row := fb.GetRows()[0]
	if row.GetCells()["cram"] != "NA12878_chr21_1x.cram" {
		t.Fatalf("unexpected CRAM cell: %#v", row.GetCells())
	}
	if row.GetCells()["cram_crai"] != "NA12878_chr21_1x.cram.crai" {
		t.Fatalf("unexpected CRAI cell: %#v", row.GetCells())
	}
	if _, ok := row.GetCells()["CRAM"]; ok {
		t.Fatalf("expected CRAM fixture probe cells to keep observed keys, got normalized CRAM key in %#v", row.GetCells())
	}
	if _, ok := row.GetCells()["CRAI"]; ok {
		t.Fatalf("expected CRAM fixture probe cells to keep observed keys, got normalized CRAI key in %#v", row.GetCells())
	}
	assertGeneratedFileBlockOutputs(t, workDir)
}

func TestSharedFSFixtureSmoke_VariantVCFIndexFixtureSpecificCurrentRuleProbe(t *testing.T) {
	root := os.Getenv("TORI_SHARED_FIXTURE_ROOT")
	if root == "" {
		root = os.Getenv("TORI_NAS_FIXTURE_ROOT")
	}
	if root == "" {
		t.Skip("TORI_SHARED_FIXTURE_ROOT is not set")
	}

	variantDir := filepath.Join(root, "variant_vcf")
	ruleSet := rules.RuleSet{
		Version:     "1",
		Delimiter:   []string{"_", "."},
		Header:      []string{"vcf_gz", "vcf_gz_csi"},
		RowRules:    rules.RowRules{MatchParts: []int{0, 1, 2}},
		ColumnRules: rules.ColumnRules{MatchParts: []int{3, 4, 5}},
		SizeRules:   rules.SizeRules{MinSize: 0, MaxSize: 104857600},
		RoleNormalization: map[string]string{
			"vcf_gz":     "VCF",
			"vcf_gz_csi": "CSI",
		},
	}
	workDir, fileNames := prepareSharedFSFixtureWorkDirWithRule(t, variantDir, ruleSet)

	preview, err := rules.GenerateResolverPreviewFromDir(workDir)
	if err != nil {
		t.Fatalf("GenerateResolverPreviewFromDir variant VCF/CSI probe error: %v", err)
	}
	if preview.RowCount != 2 || len(preview.Rows) != 2 {
		t.Fatalf("expected 2 VCF preview rows, got %#v", preview)
	}
	if preview.SourceFileCount != 4 || preview.ObservedRoleCount != 4 || preview.UnresolvedRoleCount != 0 {
		t.Fatalf("unexpected VCF preview summary: %#v", preview)
	}
	for _, row := range preview.Rows {
		if row.RoleNormalization[0].NormalizedRole != "VCF" || row.RoleNormalization[1].NormalizedRole != "CSI" {
			t.Fatalf("unexpected VCF normalization preview: %#v", row.RoleNormalization)
		}
	}
	schemaPreview := rules.BuildSchemaValidationPreview(preview, ruleSet)
	if schemaPreview.MissingRequiredRoleCount != 0 || schemaPreview.ExtraObservedRoleCount != 0 || schemaPreview.UnresolvedObservedRoleCount != 0 {
		t.Fatalf("expected VCF fixture-specific schema preview to have no candidates, got %#v", schemaPreview)
	}
	pairingPreview := rules.BuildPrimaryIndexPairingPreview(preview, "VCF", "CSI")
	if pairingPreview.EntryCount != 0 {
		t.Fatalf("expected complete VCF fixture primary/index pairing preview to have no candidates, got %#v", pairingPreview)
	}
	assertNoGeneratedPreviewOutputs(t, workDir)

	fb, err := GenerateFileBlock(workDir, fileNames)
	if err != nil {
		t.Fatalf("GenerateFileBlock variant VCF/CSI probe error: %v", err)
	}
	if len(fb.GetColumnHeaders()) != 2 || fb.GetColumnHeaders()[0] != "vcf_gz" || fb.GetColumnHeaders()[1] != "vcf_gz_csi" {
		t.Fatalf("unexpected VCF probe headers: %#v", fb.GetColumnHeaders())
	}
	if len(fb.GetRows()) != 2 {
		t.Fatalf("expected 2 variant VCF/CSI rows, got %d", len(fb.GetRows()))
	}
	for _, row := range fb.GetRows() {
		if _, ok := row.GetCells()["VCF"]; ok {
			t.Fatalf("expected VCF fixture probe cells to keep observed keys, got normalized VCF key in %#v", row.GetCells())
		}
		if _, ok := row.GetCells()["CSI"]; ok {
			t.Fatalf("expected VCF fixture probe cells to keep observed keys, got normalized CSI key in %#v", row.GetCells())
		}
	}
	assertGeneratedFileBlockOutputs(t, workDir)
}

func TestSharedFSFixtureSmoke_VariantBCFIndexFixtureSpecificCurrentRuleProbe(t *testing.T) {
	root := os.Getenv("TORI_SHARED_FIXTURE_ROOT")
	if root == "" {
		root = os.Getenv("TORI_NAS_FIXTURE_ROOT")
	}
	if root == "" {
		t.Skip("TORI_SHARED_FIXTURE_ROOT is not set")
	}

	variantDir := filepath.Join(root, "variant_bcf")
	ruleSet := rules.RuleSet{
		Version:     "1",
		Delimiter:   []string{"_", "."},
		Header:      []string{"bcf", "bcf_csi"},
		RowRules:    rules.RowRules{MatchParts: []int{0, 1}},
		ColumnRules: rules.ColumnRules{MatchParts: []int{2, 3}},
		SizeRules:   rules.SizeRules{MinSize: 0, MaxSize: 104857600},
		RoleNormalization: map[string]string{
			"bcf":     "BCF",
			"bcf_csi": "CSI",
		},
	}
	workDir, fileNames := prepareSharedFSFixtureWorkDirWithRuleAndSelectedFiles(t, variantDir, ruleSet, []string{
		"NA12878_chr21.bcf",
		"NA12878_chr21.bcf.csi",
	})

	preview, err := rules.GenerateResolverPreviewFromDir(workDir)
	if err != nil {
		t.Fatalf("GenerateResolverPreviewFromDir variant BCF/CSI probe error: %v", err)
	}
	if preview.RowCount != 1 || len(preview.Rows) != 1 {
		t.Fatalf("expected 1 BCF preview row, got %#v", preview)
	}
	if preview.SourceFileCount != 2 || preview.ObservedRoleCount != 2 || preview.UnresolvedRoleCount != 0 {
		t.Fatalf("unexpected BCF preview summary: %#v", preview)
	}
	if preview.Rows[0].RoleNormalization[0].NormalizedRole != "BCF" || preview.Rows[0].RoleNormalization[1].NormalizedRole != "CSI" {
		t.Fatalf("unexpected BCF normalization preview: %#v", preview.Rows[0].RoleNormalization)
	}
	schemaPreview := rules.BuildSchemaValidationPreview(preview, ruleSet)
	if schemaPreview.MissingRequiredRoleCount != 0 || schemaPreview.ExtraObservedRoleCount != 0 || schemaPreview.UnresolvedObservedRoleCount != 0 {
		t.Fatalf("expected BCF fixture-specific schema preview to have no candidates, got %#v", schemaPreview)
	}
	pairingPreview := rules.BuildPrimaryIndexPairingPreview(preview, "BCF", "CSI")
	if pairingPreview.EntryCount != 0 {
		t.Fatalf("expected complete BCF fixture primary/index pairing preview to have no candidates, got %#v", pairingPreview)
	}
	assertNoGeneratedPreviewOutputs(t, workDir)

	fb, err := GenerateFileBlock(workDir, fileNames)
	if err != nil {
		t.Fatalf("GenerateFileBlock variant BCF/CSI probe error: %v", err)
	}
	if len(fb.GetColumnHeaders()) != 2 || fb.GetColumnHeaders()[0] != "bcf" || fb.GetColumnHeaders()[1] != "bcf_csi" {
		t.Fatalf("unexpected BCF probe headers: %#v", fb.GetColumnHeaders())
	}
	if len(fb.GetRows()) != 1 {
		t.Fatalf("expected 1 variant BCF/CSI row, got %d", len(fb.GetRows()))
	}
	row := fb.GetRows()[0]
	if row.GetCells()["bcf"] != "NA12878_chr21.bcf" {
		t.Fatalf("unexpected BCF cell: %#v", row.GetCells())
	}
	if row.GetCells()["bcf_csi"] != "NA12878_chr21.bcf.csi" {
		t.Fatalf("unexpected BCF CSI cell: %#v", row.GetCells())
	}
	assertGeneratedFileBlockOutputs(t, workDir)
}

func TestSharedFSFixtureSmoke_ReferenceFASTAIndexFixtureSpecificCurrentRuleProbe(t *testing.T) {
	root := os.Getenv("TORI_SHARED_FIXTURE_ROOT")
	if root == "" {
		root = os.Getenv("TORI_NAS_FIXTURE_ROOT")
	}
	if root == "" {
		t.Skip("TORI_SHARED_FIXTURE_ROOT is not set")
	}

	referenceDir := filepath.Join(root, "reference_annotation")
	ruleSet := rules.RuleSet{
		Version:     "1",
		Delimiter:   []string{"_", "."},
		Header:      []string{"fasta", "fasta_fai"},
		RowRules:    rules.RowRules{MatchParts: []int{0, 1}},
		ColumnRules: rules.ColumnRules{MatchParts: []int{2, 3}},
		SizeRules:   rules.SizeRules{MinSize: 0, MaxSize: 104857600},
		RoleNormalization: map[string]string{
			"fasta":     "FASTA",
			"fasta_fai": "FAI",
		},
	}
	workDir, fileNames := prepareSharedFSFixtureWorkDirWithRuleAndSelectedFiles(t, referenceDir, ruleSet, []string{
		"sarscov2_genome.fasta",
		"sarscov2_genome.fasta.fai",
	})

	preview, err := rules.GenerateResolverPreviewFromDir(workDir)
	if err != nil {
		t.Fatalf("GenerateResolverPreviewFromDir reference FASTA/FAI probe error: %v", err)
	}
	if preview.RowCount != 1 || len(preview.Rows) != 1 {
		t.Fatalf("expected 1 FASTA preview row, got %#v", preview)
	}
	if preview.SourceFileCount != 2 || preview.ObservedRoleCount != 2 || preview.UnresolvedRoleCount != 0 {
		t.Fatalf("unexpected FASTA preview summary: %#v", preview)
	}
	if preview.Rows[0].RoleNormalization[0].NormalizedRole != "FASTA" || preview.Rows[0].RoleNormalization[1].NormalizedRole != "FAI" {
		t.Fatalf("unexpected FASTA normalization preview: %#v", preview.Rows[0].RoleNormalization)
	}
	schemaPreview := rules.BuildSchemaValidationPreview(preview, ruleSet)
	if schemaPreview.MissingRequiredRoleCount != 0 || schemaPreview.ExtraObservedRoleCount != 0 || schemaPreview.UnresolvedObservedRoleCount != 0 {
		t.Fatalf("expected FASTA fixture-specific schema preview to have no candidates, got %#v", schemaPreview)
	}
	pairingPreview := rules.BuildPrimaryIndexPairingPreview(preview, "FASTA", "FAI")
	if pairingPreview.EntryCount != 0 {
		t.Fatalf("expected complete FASTA fixture primary/index pairing preview to have no candidates, got %#v", pairingPreview)
	}
	assertNoGeneratedPreviewOutputs(t, workDir)

	fb, err := GenerateFileBlock(workDir, fileNames)
	if err != nil {
		t.Fatalf("GenerateFileBlock reference FASTA/FAI probe error: %v", err)
	}
	if len(fb.GetColumnHeaders()) != 2 || fb.GetColumnHeaders()[0] != "fasta" || fb.GetColumnHeaders()[1] != "fasta_fai" {
		t.Fatalf("unexpected FASTA probe headers: %#v", fb.GetColumnHeaders())
	}
	if len(fb.GetRows()) != 1 {
		t.Fatalf("expected 1 reference FASTA/FAI row, got %d", len(fb.GetRows()))
	}
	row := fb.GetRows()[0]
	if row.GetCells()["fasta"] != "sarscov2_genome.fasta" {
		t.Fatalf("unexpected FASTA cell: %#v", row.GetCells())
	}
	if row.GetCells()["fasta_fai"] != "sarscov2_genome.fasta.fai" {
		t.Fatalf("unexpected FAI cell: %#v", row.GetCells())
	}
	assertGeneratedFileBlockOutputs(t, workDir)
}

func prepareSharedFSFixtureWorkDir(t *testing.T, sourceDir string) (string, []string) {
	t.Helper()

	exclusions := []string{"rule.json", "invalid_files", "fileblock.csv", "*.pb"}
	fileNames, err := rules.ListFilesExclude(sourceDir, exclusions)
	if err != nil {
		t.Fatalf("list shared filesystem fixture files from %s: %v", sourceDir, err)
	}

	ruleData, err := os.ReadFile(filepath.Join(sourceDir, "rule.json"))
	if err != nil {
		t.Fatalf("read shared filesystem fixture rule.json from %s: %v", sourceDir, err)
	}

	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "rule.json"), ruleData, 0644); err != nil {
		t.Fatalf("write temp rule.json: %v", err)
	}
	writeFixtureFileNamePlaceholders(t, workDir, fileNames)

	return workDir, fileNames
}

func prepareSharedFSFixtureWorkDirWithRule(t *testing.T, sourceDir string, ruleSet rules.RuleSet) (string, []string) {
	t.Helper()

	exclusions := []string{"rule.json", "invalid_files", "fileblock.csv", "*.pb"}
	fileNames, err := rules.ListFilesExclude(sourceDir, exclusions)
	if err != nil {
		t.Fatalf("list shared filesystem fixture files from %s: %v", sourceDir, err)
	}

	ruleData, err := json.MarshalIndent(ruleSet, "", "  ")
	if err != nil {
		t.Fatalf("marshal probe rule.json: %v", err)
	}

	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "rule.json"), ruleData, 0644); err != nil {
		t.Fatalf("write temp probe rule.json: %v", err)
	}
	writeFixtureFileNamePlaceholders(t, workDir, fileNames)

	return workDir, fileNames
}

func prepareSharedFSFixtureWorkDirWithRuleAndSelectedFiles(t *testing.T, sourceDir string, ruleSet rules.RuleSet, fileNames []string) (string, []string) {
	t.Helper()

	for _, fileName := range fileNames {
		if _, err := os.Stat(filepath.Join(sourceDir, fileName)); err != nil {
			t.Fatalf("stat selected shared filesystem fixture file %s: %v", filepath.Join(sourceDir, fileName), err)
		}
	}

	ruleData, err := json.MarshalIndent(ruleSet, "", "  ")
	if err != nil {
		t.Fatalf("marshal probe rule.json: %v", err)
	}

	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "rule.json"), ruleData, 0644); err != nil {
		t.Fatalf("write temp probe rule.json: %v", err)
	}
	writeFixtureFileNamePlaceholders(t, workDir, fileNames)

	return workDir, fileNames
}

func writeFixtureFileNamePlaceholders(t *testing.T, workDir string, fileNames []string) {
	t.Helper()

	for _, name := range fileNames {
		if err := os.WriteFile(filepath.Join(workDir, name), []byte("fixture"), 0644); err != nil {
			t.Fatalf("write temp fixture placeholder %s: %v", name, err)
		}
	}
}

func loadSharedFSFixtureRuleSet(t *testing.T, workDir string) rules.RuleSet {
	t.Helper()

	ruleSet, err := rules.LoadRuleSetFromFile(workDir)
	if err != nil {
		t.Fatalf("load temp fixture rule set from %s: %v", workDir, err)
	}
	return ruleSet
}

func assertNoGeneratedPreviewOutputs(t *testing.T, workDir string) {
	t.Helper()

	if _, err := os.Stat(filepath.Join(workDir, "fileblock.csv")); err == nil {
		t.Fatalf("expected preview path not to write fileblock.csv")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat preview fileblock.csv: %v", err)
	}

	pbMatches, err := filepath.Glob(filepath.Join(workDir, "*.pb"))
	if err != nil {
		t.Fatalf("glob preview protobuf outputs: %v", err)
	}
	if len(pbMatches) != 0 {
		t.Fatalf("expected preview path not to write protobuf outputs, got %v", pbMatches)
	}

	invalidMatches, err := filepath.Glob(filepath.Join(workDir, "invalid_files_*.txt"))
	if err != nil {
		t.Fatalf("glob preview invalid reports: %v", err)
	}
	if len(invalidMatches) != 0 {
		t.Fatalf("expected preview path not to write invalid reports, got %v", invalidMatches)
	}
}

func assertGeneratedFileBlockOutputs(t *testing.T, workDir string) {
	t.Helper()

	csvPath := filepath.Join(workDir, "fileblock.csv")
	if _, err := os.Stat(csvPath); err != nil {
		t.Fatalf("expected generated fileblock.csv at %s: %v", csvPath, err)
	}

	pbPath := filepath.Join(workDir, filepath.Base(workDir)+"files.pb")
	if _, err := os.Stat(pbPath); err != nil {
		t.Fatalf("expected generated protobuf output at %s: %v", pbPath, err)
	}
}
