package db

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HeaInSeo/tori/rules"
)

// TDI-I3M minimum tests 1–9. Each test name carries its packet test number.

func i3mRuleSet(t *testing.T) rules.RuleSet {
	t.Helper()
	rs, err := rules.RuleSetFromCanonical(mustCanonicalRule(t))
	if err != nil {
		t.Fatalf("rule set: %v", err)
	}
	return rs
}

func mustCanonicalRule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "rule.json"), []byte(ruleJSON), 0o600); err != nil {
		t.Fatalf("write rule: %v", err)
	}
	rs, err := rules.LoadRuleSetFromFile(dir)
	if err != nil {
		t.Fatalf("load rule: %v", err)
	}
	canonical, _, err := rules.FreezeRuleSet(rs)
	if err != nil {
		t.Fatalf("freeze rule: %v", err)
	}
	return canonical
}

// i3mManifest builds a manifest for one folder from file names, in the given order.
func i3mManifest(t *testing.T, folder string, files []string) PublicationSemanticManifest {
	t.Helper()
	subjects, conflicts := rules.PublicationSubjects(files, i3mRuleSet(t))
	if len(conflicts) != 0 {
		t.Fatalf("unexpected conflicts: %+v", conflicts)
	}
	mf := ManifestFolder{Path: folder, ClassificationRevisionID: "cls-r1"}
	for _, s := range subjects {
		ms := ManifestSubject{SubjectKey: s.SubjectKey, Components: s.Components}
		for _, m := range s.Members {
			ms.Members = append(ms.Members, ManifestMember{
				ObservedKey: m.ObservedKey, NormalizedRole: m.NormalizedRole, FileName: m.FileName, Integrity: "size:1",
			})
		}
		mf.Subjects = append(mf.Subjects, ms)
	}
	return PublicationSemanticManifest{SourceID: "src-test", SourceRevisionID: "rev-1", Folders: []ManifestFolder{mf}}
}

func mustPublish(t *testing.T, db *sql.DB, opID string, m PublicationSemanticManifest) PublicationResult {
	t.Helper()
	res, err := Publish(context.Background(), db, PublicationRequest{OperationID: opID, Manifest: m})
	if err != nil {
		t.Fatalf("Publish(%s): %v", opID, err)
	}
	if res.GenerationID == "" {
		t.Fatalf("Publish(%s) returned no Generation", opID)
	}
	return res
}

func countRows(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM "+table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func withPublicationCrash(t *testing.T, boundary string) {
	t.Helper()
	publicationCrashHookForTest = func(b string) error {
		if b == boundary {
			return errors.New("simulated crash at " + b)
		}
		return nil
	}
	t.Cleanup(func() { publicationCrashHookForTest = nil })
}

var i3mFilesAB = append(pairFiles("A"), pairFiles("B")...)

// Test 1: same operation + same semantics retried after ack loss → same Generation.
func TestI3M_T01_RetryAfterAckLossSameGeneration(t *testing.T) {
	db := newAcceptanceDB(t)
	m := i3mManifest(t, "runA", i3mFilesAB)
	first := mustPublish(t, db, "op-1", m)
	// The ack of `first` is lost; the caller retries the same request.
	retry := mustPublish(t, db, "op-1", m)
	if retry.GenerationID != first.GenerationID || !retry.Reconciled {
		t.Fatalf("retry = %+v, want same Generation %s reconciled", retry, first.GenerationID)
	}
	if n := countRows(t, db, "publication_generations"); n != 1 {
		t.Fatalf("generations = %d, want 1", n)
	}
}

// Test 2: same operation ID + changed semantics → conflict, nothing written.
func TestI3M_T02_SameOperationChangedSemanticsConflicts(t *testing.T) {
	db := newAcceptanceDB(t)
	mustPublish(t, db, "op-1", i3mManifest(t, "runA", pairFiles("A")))
	_, err := Publish(context.Background(), db, PublicationRequest{OperationID: "op-1", Manifest: i3mManifest(t, "runA", i3mFilesAB)})
	if !errors.Is(err, ErrPublicationConflict) {
		t.Fatalf("err = %v, want ErrPublicationConflict", err)
	}
	if n := countRows(t, db, "publication_generations"); n != 1 {
		t.Fatalf("generations = %d, want 1", n)
	}
	if n := countRows(t, db, "publication_manifests"); n != 1 {
		t.Fatalf("manifests = %d, want 1 (conflicting manifest must not be recorded)", n)
	}
}

// Test 3: two independent operations with an identical manifest → same Generation.
func TestI3M_T03_IndependentOperationsConverge(t *testing.T) {
	db := newAcceptanceDB(t)
	a := mustPublish(t, db, "cycle-1", i3mManifest(t, "runA", i3mFilesAB))
	b := mustPublish(t, db, "cycle-2", i3mManifest(t, "runA", i3mFilesAB))
	if a.GenerationID != b.GenerationID || a.ManifestID != b.ManifestID {
		t.Fatalf("independent operations diverged: %+v vs %+v", a, b)
	}
	if b.Reconciled {
		t.Fatalf("cycle-2 is a new operation, not a retry")
	}
	if n := countRows(t, db, "publication_generations"); n != 1 {
		t.Fatalf("generations = %d, want 1", n)
	}
	if n := countRows(t, db, "publication_operations"); n != 2 {
		t.Fatalf("operations = %d, want 2", n)
	}
}

// Test 4: file/list order changes do not change the manifest or Generation.
func TestI3M_T04_OrderIndependence(t *testing.T) {
	forward := i3mManifest(t, "runA", i3mFilesAB)
	reversed := make([]string, len(i3mFilesAB))
	for i, f := range i3mFilesAB {
		reversed[len(i3mFilesAB)-1-i] = f
	}
	backward := i3mManifest(t, "runA", reversed)
	// Also permute folder, subject and member order in the manifest itself.
	two := forward
	two.Folders = append([]ManifestFolder{{Path: "runB", ClassificationRevisionID: "cls-r1", Subjects: backward.Folders[0].Subjects}}, forward.Folders...)
	twoPermuted := backward
	subj := append([]ManifestSubject(nil), backward.Folders[0].Subjects...)
	for i, j := 0, len(subj)-1; i < j; i, j = i+1, j-1 {
		subj[i], subj[j] = subj[j], subj[i]
	}
	for i := range subj {
		mem := append([]ManifestMember(nil), subj[i].Members...)
		for a, b := 0, len(mem)-1; a < b; a, b = a+1, b-1 {
			mem[a], mem[b] = mem[b], mem[a]
		}
		subj[i].Members = mem
	}
	twoPermuted.Folders = []ManifestFolder{forward.Folders[0], {Path: "runB", ClassificationRevisionID: "cls-r1", Subjects: subj}}

	_, id1, err := forward.ManifestID()
	if err != nil {
		t.Fatal(err)
	}
	_, id2, err := backward.ManifestID()
	if err != nil {
		t.Fatal(err)
	}
	if id1 != id2 {
		t.Fatalf("file order changed ManifestID: %s vs %s", id1, id2)
	}
	_, id3, err := two.ManifestID()
	if err != nil {
		t.Fatal(err)
	}
	_, id4, err := twoPermuted.ManifestID()
	if err != nil {
		t.Fatal(err)
	}
	if id3 != id4 {
		t.Fatalf("folder/subject/member order changed ManifestID: %s vs %s", id3, id4)
	}
	db := newAcceptanceDB(t)
	if g1, g2 := mustPublish(t, db, "op-f", two), mustPublish(t, db, "op-p", twoPermuted); g1.GenerationID != g2.GenerationID {
		t.Fatalf("order changed Generation: %s vs %s", g1.GenerationID, g2.GenerationID)
	}
}

// Test 5: an unrelated subject addition makes a distinct manifest/Generation while the
// prior subject's coordinate stays stable.
func TestI3M_T05_UnrelatedAdditionDistinctGenerationStableCoordinate(t *testing.T) {
	db := newAcceptanceDB(t)
	before := mustPublish(t, db, "op-1", i3mManifest(t, "runA", pairFiles("A")))
	after := mustPublish(t, db, "op-2", i3mManifest(t, "runA", i3mFilesAB))
	if before.GenerationID == after.GenerationID || before.ManifestID == after.ManifestID {
		t.Fatalf("subject addition did not produce a distinct Generation")
	}
	g1, _, err := GetGeneration(context.Background(), db, before.GenerationID)
	if err != nil {
		t.Fatal(err)
	}
	g2, _, err := GetGeneration(context.Background(), db, after.GenerationID)
	if err != nil {
		t.Fatal(err)
	}
	if len(g1.Subjects) != 1 || len(g2.Subjects) != 2 {
		t.Fatalf("subjects = %d/%d, want 1/2", len(g1.Subjects), len(g2.Subjects))
	}
	var found bool
	for _, s := range g2.Subjects {
		if s.FolderPath == g1.Subjects[0].FolderPath && s.SubjectKey == g1.Subjects[0].SubjectKey {
			found = s.SubjectCanonical == g1.Subjects[0].SubjectCanonical
		}
	}
	if !found {
		t.Fatalf("prior subject %s not stable in the new Generation", g1.Subjects[0].SubjectKey)
	}
}

// Test 6: a crash at each persistence boundary cannot duplicate a Generation, and the
// retry of the same operation converges.
func TestI3M_T06_CrashAtEachBoundaryNoDuplicate(t *testing.T) {
	for _, boundary := range []string{boundaryAfterAccept, boundaryBeforeMintEnd, boundaryAfterMint} {
		t.Run(boundary, func(t *testing.T) {
			db := newAcceptanceDB(t)
			m := i3mManifest(t, "runA", i3mFilesAB)
			withPublicationCrash(t, boundary)
			if _, err := Publish(context.Background(), db, PublicationRequest{OperationID: "op-1", Manifest: m}); err == nil {
				t.Fatalf("expected simulated crash at %s", boundary)
			}
			publicationCrashHookForTest = nil
			// A different operation with the same manifest may also arrive first.
			other := mustPublish(t, db, "op-2", m)
			retry := mustPublish(t, db, "op-1", m)
			if retry.GenerationID != other.GenerationID {
				t.Fatalf("retry Generation %s != %s", retry.GenerationID, other.GenerationID)
			}
			if !retry.Reconciled {
				t.Fatalf("retry after %s should reconcile the accepted operation", boundary)
			}
			if n := countRows(t, db, "publication_generations"); n != 1 {
				t.Fatalf("generations = %d, want 1", n)
			}
			if n := countRows(t, db, "publication_generation_subjects"); n != 2 {
				t.Fatalf("generation subjects = %d, want 2", n)
			}
		})
	}
}

// Test 7: Generation rows and manifests are immutable after acceptance.
func TestI3M_T07_GenerationImmutable(t *testing.T) {
	ctx := context.Background()
	db := newAcceptanceDB(t)
	res := mustPublish(t, db, "op-1", i3mManifest(t, "runA", i3mFilesAB))
	attempts := []string{
		"UPDATE publication_generations SET manifest_id = 'x'",
		"DELETE FROM publication_generations",
		"UPDATE publication_generation_subjects SET subject_canonical = '{}'",
		"DELETE FROM publication_generation_subjects",
		"INSERT INTO publication_generation_subjects VALUES ('" + res.GenerationID + "', 'runA', 'extra', 'cls-r1', '{}')",
		"UPDATE publication_manifests SET canonical = '{}'",
		"DELETE FROM publication_manifests",
		"UPDATE publication_operations SET manifest_id = 'x'",
		"UPDATE publication_operations SET generation_id = 'gen-other'",
		"DELETE FROM publication_operations",
	}
	for _, q := range attempts {
		if _, err := db.ExecContext(ctx, q); err == nil {
			t.Errorf("mutation succeeded: %s", q)
		}
	}
	g, ok, err := GetGeneration(ctx, db, res.GenerationID)
	if err != nil || !ok {
		t.Fatalf("GetGeneration: ok=%v err=%v", ok, err)
	}
	if len(g.Subjects) != 2 || g.ManifestID != res.ManifestID {
		t.Fatalf("Generation changed: %+v", g)
	}
}

// Test 8: a legacy DataBlock rewrite does not mutate Generation truth. The manifest is
// built from the accepted snapshot, not datablock.pb.
func TestI3M_T08_DataBlockRewriteDoesNotMutateGeneration(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	db := newAcceptanceDB(t)
	writeRuleFolder(t, root, "runA", i3mFilesAB...)
	acceptBaseline(t, db, root)

	m, err := BuildAcceptedPublicationManifest(ctx, db, root)
	if err != nil {
		t.Fatalf("BuildAcceptedPublicationManifest: %v", err)
	}
	res := mustPublish(t, db, "op-1", m)
	before, _, err := GetGeneration(ctx, db, res.GenerationID)
	if err != nil {
		t.Fatal(err)
	}

	// Rewrite the compatibility projection: garbage, then a rebuild from the accepted DB.
	dbPath := filepath.Join(root, "datablock.pb")
	if err := os.WriteFile(dbPath, []byte("not a datablock"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := regenerateProjectionFromDB(ctx, db, root, nil); err != nil {
		t.Fatalf("regenerate projection: %v", err)
	}

	after, _, err := GetGeneration(ctx, db, res.GenerationID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Subjects) != len(before.Subjects) || after.ManifestID != before.ManifestID {
		t.Fatalf("Generation changed after DataBlock rewrite")
	}
	for i := range before.Subjects {
		if before.Subjects[i] != after.Subjects[i] {
			t.Fatalf("subject %d changed: %+v → %+v", i, before.Subjects[i], after.Subjects[i])
		}
	}
	m2, err := BuildAcceptedPublicationManifest(ctx, db, root)
	if err != nil {
		t.Fatal(err)
	}
	if _, id, _ := m2.ManifestID(); id != res.ManifestID {
		t.Fatalf("manifest after DataBlock rewrite = %s, want %s", id, res.ManifestID)
	}
	if strings.Contains(res.GenerationID, "datablock") {
		t.Fatalf("Generation identity derived from the projection")
	}
}

// Test 9: publication has no Auto-Run/Run side effect: it creates only publication
// tables and writes nothing under the source root.
func TestI3M_T09_NoRunSideEffect(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	db := newAcceptanceDB(t)
	writeRuleFolder(t, root, "runA", i3mFilesAB...)
	acceptBaseline(t, db, root)

	tablesBefore := tableNames(t, db)
	filesBefore := treeListing(t, root)

	m, err := BuildAcceptedPublicationManifest(ctx, db, root)
	if err != nil {
		t.Fatal(err)
	}
	mustPublish(t, db, "op-1", m)

	for name := range tableNames(t, db) {
		if _, ok := tablesBefore[name]; ok {
			continue
		}
		lower := strings.ToLower(name)
		if !strings.HasPrefix(lower, "publication_") {
			t.Errorf("publish created non-publication table %s", name)
		}
		for _, forbidden := range []string{"run", "autorun", "auto_run", "sori"} {
			if strings.Contains(strings.TrimPrefix(lower, "publication_"), forbidden) {
				t.Errorf("publish created %s table %s", forbidden, name)
			}
		}
	}
	if after := treeListing(t, root); after != filesBefore {
		t.Fatalf("publish changed the source tree:\nbefore %s\nafter  %s", filesBefore, after)
	}
}

// An unaccepted snapshot cannot be published.
func TestI3M_BuildManifestRequiresCleanAcceptedSnapshot(t *testing.T) {
	db := newAcceptanceDB(t)
	if _, err := BuildAcceptedPublicationManifest(context.Background(), db, t.TempDir()); !errors.Is(err, ErrPublicationNotAccepted) {
		t.Fatalf("err = %v, want ErrPublicationNotAccepted", err)
	}
}

// Relocating the endpoint does not change the manifest (folder paths are root-relative).
func TestI3M_ManifestIsRootRelative(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	db := newAcceptanceDB(t)
	writeRuleFolder(t, root, "runA", i3mFilesAB...)
	acceptBaseline(t, db, root)
	m, err := BuildAcceptedPublicationManifest(ctx, db, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Folders) != 1 || m.Folders[0].Path != "runA" || len(m.Folders[0].Subjects) != 2 {
		t.Fatalf("manifest folders = %+v", m.Folders)
	}
	for _, s := range m.Folders[0].Subjects {
		for _, mem := range s.Members {
			if !strings.HasPrefix(mem.Integrity, "size:1;accepted-change:") {
				t.Fatalf("member %s integrity = %q", mem.FileName, mem.Integrity)
			}
		}
	}
}

// Folder paths are bound to the access root recorded for the accepted snapshot: an
// ancestor (or any other) caller root is refused instead of minting a second ManifestID
// for the same accepted data, and an equivalent spelling of the recorded root still
// yields the same ManifestID. Refusals publish nothing.
func TestI3M_ManifestBoundToRecordedRoot(t *testing.T) {
	ctx := context.Background()
	parent := t.TempDir()
	root := filepath.Join(parent, "source")
	db := newAcceptanceDB(t)
	writeRuleFolder(t, root, "runA", i3mFilesAB...)
	acceptBaseline(t, db, root)
	want := mustManifestID(t, db, root)
	tables := []string{"publication_manifests", "publication_generations", "publication_generation_subjects", "publication_operations"}
	for name, other := range map[string]string{"ancestor": parent, "sibling": t.TempDir(), "child": filepath.Join(root, "runA")} {
		if m, err := BuildAcceptedPublicationManifest(ctx, db, other); !errors.Is(err, ErrPublicationNotAccepted) {
			t.Fatalf("%s root %s: manifest %+v err = %v, want ErrPublicationNotAccepted", name, other, m.Folders, err)
		}
	}
	for _, table := range tables {
		if tableExists(t, db, table) {
			t.Fatalf("refused manifest builds created %s", table)
		}
	}
	if got := mustManifestID(t, db, root+string(filepath.Separator)+"."+string(filepath.Separator)); got != want {
		t.Fatalf("equivalent spelling of the recorded root changed ManifestID %s -> %s", want, got)
	}
}

func tableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?", name).Scan(&n); err != nil {
		t.Fatalf("lookup table %s: %v", name, err)
	}
	return n == 1
}

func mustManifestID(t *testing.T, db *sql.DB, root string) string {
	t.Helper()
	m, err := BuildAcceptedPublicationManifest(context.Background(), db, root)
	if err != nil {
		t.Fatalf("BuildAcceptedPublicationManifest: %v", err)
	}
	_, id, err := m.ManifestID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// replaceSameLength rewrites path with different bytes of the same length and moves its
// mtime, so SyncFolders observes and accepts it as modified.
func replaceSameLength(t *testing.T, path string, content []byte) {
	t.Helper()
	info, _ := statOf(t, path)
	if int64(len(content)) != info.Size() {
		t.Fatalf("precondition: replacement must keep size %d", info.Size())
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("rewrite %s: %v", path, err)
	}
	moved := info.ModTime().Add(2 * time.Second)
	if err := os.Chtimes(path, moved, moved); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
}

// Codex P1 (content identity): an accepted same-length content change must not reuse the
// prior ManifestID/Generation, while an unchanged re-scan still converges.
func TestI3M_SameLengthChangeDistinctManifest(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dir := writeRuleFolder(t, root, "runA", i3mFilesAB...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)
	before := mustManifestID(t, db, root)

	replaceSameLength(t, filepath.Join(dir, i3mFilesAB[0]), []byte("y"))
	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil || res.Outcome != OutcomeAcceptedUpdate {
		t.Fatalf("same-length change: expected accepted-update, got %v (%s) err=%v", res.Outcome, res.Reason, err)
	}
	after := mustManifestID(t, db, root)
	if after == before {
		t.Fatalf("same-length content change reused ManifestID %s", before)
	}

	res, err = SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil || res.Outcome != OutcomeUnchanged {
		t.Fatalf("settle: expected unchanged, got %v (%s) err=%v", res.Outcome, res.Reason, err)
	}
	if again := mustManifestID(t, db, root); again != after {
		t.Fatalf("unchanged re-scan changed ManifestID: %s vs %s", again, after)
	}

	mb, err := BuildAcceptedPublicationManifest(ctx, db, root)
	if err != nil {
		t.Fatal(err)
	}
	g := mustPublish(t, db, "op-after", mb)
	if g.ManifestID == before {
		t.Fatalf("published Generation reuses the pre-change manifest")
	}
}

// manifestHasFile reports whether any member of the accepted manifest is fileName.
func manifestHasFile(t *testing.T, db *sql.DB, root, fileName string) bool {
	t.Helper()
	m, err := BuildAcceptedPublicationManifest(context.Background(), db, root)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range m.Folders {
		for _, s := range f.Subjects {
			for _, mem := range s.Members {
				if mem.FileName == fileName {
					return true
				}
			}
		}
	}
	return false
}

// Codex P1 (acceptance boundary): re-running the seed over a clean accepted DB inserts a new
// file of an established folder without an acceptance transition. That row must not enter a
// publication manifest until the canonical clean transition accepts it.
func TestI3M_ReseededRowIneligibleUntilAccepted(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dir := writeRuleFolder(t, root, "runA", pairFiles("A")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)
	before := mustManifestID(t, db, root)

	seeded := pairFiles("C")
	for _, f := range seeded {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := StoreFilesFolderInfo(ctx, db, dir, acceptanceExclusions); err != nil {
		t.Fatalf("re-seed: %v", err)
	}
	if got := mustManifestID(t, db, root); got != before {
		t.Fatalf("re-seed changed the accepted ManifestID: %s vs %s", got, before)
	}
	if manifestHasFile(t, db, root, seeded[0]) {
		t.Fatalf("re-seeded %s entered the manifest without an acceptance transition", seeded[0])
	}

	// An unchanged re-scan is not an acceptance transition: still ineligible.
	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil || res.Outcome != OutcomeUnchanged {
		t.Fatalf("re-scan: expected unchanged, got %v (%s) err=%v", res.Outcome, res.Reason, err)
	}
	if got := mustManifestID(t, db, root); got != before {
		t.Fatalf("unchanged re-scan changed the accepted ManifestID: %s vs %s", got, before)
	}

	// The next canonical acceptance promotes the re-seeded rows with the rebuilt projection.
	if err := os.WriteFile(filepath.Join(dir, pairFiles("A")[0]), []byte("zz"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err = SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil || res.Outcome != OutcomeAcceptedUpdate {
		t.Fatalf("accept: expected accepted-update, got %v (%s) err=%v", res.Outcome, res.Reason, err)
	}
	for _, f := range seeded {
		if !manifestHasFile(t, db, root, f) {
			t.Fatalf("%s not eligible after the canonical acceptance", f)
		}
	}
}

// Guardrail P2-1 (restore isolation): restoring a missing projection re-commits clean at the
// same accepted_version ("no version bump"). That is not an acceptance, so re-seeded rows must
// stay ineligible and the accepted version must keep its single manifest.
func TestI3M_RestoreKeepsReseededRowsIneligible(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dir := writeRuleFolder(t, root, "runA", pairFiles("A")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)
	before := mustManifestID(t, db, root)
	version, err := metaGetInt(ctx, db, metaKeyAcceptedVersion)
	if err != nil {
		t.Fatal(err)
	}

	seeded := pairFiles("C")
	for _, f := range seeded {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := StoreFilesFolderInfo(ctx, db, dir, acceptanceExclusions); err != nil {
		t.Fatalf("re-seed: %v", err)
	}
	if err := os.Remove(filepath.Join(root, "datablock.pb")); err != nil {
		t.Fatal(err)
	}
	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil || !strings.Contains(res.Reason, "restored missing projection") {
		t.Fatalf("restore: got %v (%s) err=%v", res.Outcome, res.Reason, err)
	}
	if v, err := metaGetInt(ctx, db, metaKeyAcceptedVersion); err != nil || v != version {
		t.Fatalf("restore moved accepted_version %d -> %d (err=%v); this test needs the no-bump path", version, v, err)
	}
	if got := mustManifestID(t, db, root); got != before {
		t.Fatalf("restore changed the manifest of accepted version %d: %s vs %s", version, got, before)
	}
	if manifestHasFile(t, db, root, seeded[0]) {
		t.Fatalf("re-seeded %s entered the manifest through a no-version-bump restore", seeded[0])
	}
}

// Every no-version-bump restore (missing projection, drift HOLD, evidence HOLD) re-commits
// clean through commitClean with target == accepted; only a version advance clears markers.
func TestI3M_CommitCleanClearsSeedMarkersOnlyOnVersionAdvance(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dir := writeRuleFolder(t, root, "runA", pairFiles("A")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)
	for _, f := range pairFiles("C") {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := StoreFilesFolderInfo(ctx, db, dir, acceptanceExclusions); err != nil {
		t.Fatalf("re-seed: %v", err)
	}
	version, err := metaGetInt(ctx, db, metaKeyAcceptedVersion)
	if err != nil {
		t.Fatal(err)
	}
	if err := commitClean(ctx, db, version); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, db, "file_unaccepted_seed"); n != 2 {
		t.Fatalf("markers after no-bump commitClean = %d, want 2", n)
	}
	if err := commitClean(ctx, db, version+1); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, db, "file_unaccepted_seed"); n != 0 {
		t.Fatalf("markers after version-advancing commitClean = %d, want 0", n)
	}
}

// Codex P1 4166471114: a re-seed that inserts a new folder under an accepted root must not
// wedge publication. The folder has no classification basis at the accepted version, so it
// is excluded until a version-advancing acceptance, and the accepted manifest stays the same.
func TestI3M_ReseededNewFolderDoesNotBlockPublication(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeRuleFolder(t, root, "runA", pairFiles("A")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)
	before := mustManifestID(t, db, root)

	dirB := writeRuleFolder(t, root, "runB", pairFiles("C")...)
	if err := StoreFilesFolderInfo(ctx, db, dirB, acceptanceExclusions); err != nil {
		t.Fatalf("re-seed new folder: %v", err)
	}
	if got := mustManifestID(t, db, root); got != before {
		t.Fatalf("re-seeded folder changed the accepted ManifestID: %s vs %s", got, before)
	}
	if n := countRows(t, db, "folder_unaccepted_seed"); n != 1 {
		t.Fatalf("folder markers after re-seed = %d, want 1", n)
	}
	m, err := BuildAcceptedPublicationManifest(ctx, db, root)
	if err != nil {
		t.Fatal(err)
	}
	mustPublish(t, db, "op-new-folder", m)
	if manifestHasFile(t, db, root, pairFiles("C")[0]) {
		t.Fatal("re-seeded folder entered the manifest without an acceptance transition")
	}

	// A re-scan that does not advance accepted_version (today a reclassify HOLD for the
	// basis-less folder) keeps the folder out and never blocks publication.
	if _, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions); err != nil {
		t.Fatalf("re-scan: %v", err)
	}
	if got := mustManifestID(t, db, root); got != before {
		t.Fatalf("re-scan changed the accepted ManifestID: %s vs %s", got, before)
	}

	// The folder marker follows the file markers: a no-version-bump commitClean keeps it,
	// a version-advancing one clears it.
	version, err := metaGetInt(ctx, db, metaKeyAcceptedVersion)
	if err != nil {
		t.Fatal(err)
	}
	if err := commitClean(ctx, db, version); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, db, "folder_unaccepted_seed"); n != 1 {
		t.Fatalf("folder markers after no-bump commitClean = %d, want 1", n)
	}
	if err := commitClean(ctx, db, version+1); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, db, "folder_unaccepted_seed"); n != 0 {
		t.Fatalf("folder markers after version-advancing commitClean = %d, want 0", n)
	}
}

// Codex P2 4166471147: the exported Publish API rejects a member without an observed key,
// file name or integrity before anything is written.
func TestI3M_PublishRejectsIncompleteMembers(t *testing.T) {
	db := newAcceptanceDB(t)
	mustPublish(t, db, "op-valid", i3mManifest(t, "runA", pairFiles("A")))
	tables := []string{"publication_manifests", "publication_generations", "publication_generation_subjects", "publication_operations"}
	before := make(map[string]int, len(tables))
	for _, table := range tables {
		before[table] = countRows(t, db, table)
	}
	for name, blank := range map[string]func(*ManifestMember){
		"observed key": func(m *ManifestMember) { m.ObservedKey = "" },
		"file name":    func(m *ManifestMember) { m.FileName = "" },
		"integrity":    func(m *ManifestMember) { m.Integrity = "" },
	} {
		m := i3mManifest(t, "runB", pairFiles("B"))
		blank(&m.Folders[0].Subjects[0].Members[0])
		_, err := Publish(context.Background(), db, PublicationRequest{OperationID: "op-" + name, Manifest: m})
		if !errors.Is(err, ErrPublicationManifestInvalid) {
			t.Fatalf("empty %s: err = %v, want ErrPublicationManifestInvalid", name, err)
		}
	}
	for _, table := range tables {
		if n := countRows(t, db, table); n != before[table] {
			t.Fatalf("%s = %d after rejected publishes, want %d", table, n, before[table])
		}
	}
}

// Codex P1 (schema validity): accepted subjects with a missing, extra or unresolved role
// are excluded from the typed projection, so they must not enter a manifest or Generation.
func TestI3M_SchemaInvalidSubjectsNotPublished(t *testing.T) {
	root := t.TempDir()
	invalid := []string{
		"M_S1_L001_R1_001.fastq.gz",
		"X_S1_L001_R1_001.fastq.gz", "X_S1_L001_R2_001.fastq.gz", "X_S1_L001_R3_001.fastq.gz",
		"U_S1_L001_R1_001.fastq.gz", "U_S1_L001_I1_001.fastq.gz",
	}
	writeRuleFolder(t, root, "runA", append(pairFiles("A"), invalid...)...)
	db := newAcceptanceDB(t)
	// Not acceptBaseline: invalid rows emit a timestamped invalid_files report, so the
	// boundary does not settle to unchanged. One accepted-update is a clean accepted state.
	ctx := context.Background()
	if err := SaveFolders(ctx, db, root, nil, acceptanceExclusions); err != nil {
		t.Fatalf("SaveFolders: %v", err)
	}
	if res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions); err != nil || res.Outcome != OutcomeAcceptedUpdate {
		t.Fatalf("baseline: expected accepted-update, got %v (%s) err=%v", res.Outcome, res.Reason, err)
	}

	m, err := BuildAcceptedPublicationManifest(ctx, db, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Folders) != 1 || len(m.Folders[0].Subjects) != 1 || len(m.Folders[0].Subjects[0].Members) != 2 {
		t.Fatalf("manifest folders = %+v, want only the valid A subject", m.Folders)
	}
	for _, f := range invalid {
		if manifestHasFile(t, db, root, f) {
			t.Fatalf("schema-invalid %s entered the manifest", f)
		}
	}
	for _, f := range pairFiles("A") {
		if !manifestHasFile(t, db, root, f) {
			t.Fatalf("valid %s missing from the manifest", f)
		}
	}
	mustPublish(t, db, "op-schema", m)
	if n := countRows(t, db, "publication_generation_subjects"); n != 1 {
		t.Fatalf("published subjects = %d, want 1", n)
	}
}

// Codex P1 (snapshot): a SyncFolders commit racing the manifest build cannot produce a hybrid
// of version-N bases and a version-N+1 inventory. The build sees exactly version N.
func TestI3M_ConcurrentSyncNoHybridManifest(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dir := writeRuleFolder(t, root, "runA", i3mFilesAB...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)
	versionN := mustManifestID(t, db, root)

	replaceSameLength(t, filepath.Join(dir, i3mFilesAB[0]), []byte("y"))
	if err := os.WriteFile(filepath.Join(dir, i3mFilesAB[1]), []byte("zz"), 0o600); err != nil {
		t.Fatal(err)
	}
	var raced bool
	publicationSnapshotHookForTest = func() {
		publicationSnapshotHookForTest = nil
		raced = true
		syncCtx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
		defer cancel()
		res, err := SyncFolders(syncCtx, db, root, nil, acceptanceExclusions)
		t.Logf("racing SyncFolders inside the snapshot read: outcome=%v err=%v", res.Outcome, err)
	}
	t.Cleanup(func() { publicationSnapshotHookForTest = nil })

	during := mustManifestID(t, db, root)
	if !raced {
		t.Fatal("snapshot hook did not run")
	}
	if during != versionN {
		t.Fatalf("manifest built during a racing sync = %s, want the version-N manifest %s (hybrid snapshot)", during, versionN)
	}

	// After the race, a sync commits version N+1 and the manifest moves as a whole.
	for i := 0; i < 2; i++ {
		if _, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions); err != nil {
			t.Fatalf("SyncFolders after race: %v", err)
		}
	}
	m, err := BuildAcceptedPublicationManifest(ctx, db, root)
	if err != nil {
		t.Fatal(err)
	}
	_, next, err := m.ManifestID()
	if err != nil {
		t.Fatal(err)
	}
	if next == versionN {
		t.Fatalf("manifest did not advance after the accepted change")
	}
	var sawNewSize bool
	for _, s := range m.Folders[0].Subjects {
		for _, mem := range s.Members {
			if mem.FileName == i3mFilesAB[1] {
				sawNewSize = strings.HasPrefix(mem.Integrity, "size:2;")
			}
		}
	}
	if !sawNewSize {
		t.Fatalf("version N+1 manifest does not carry the accepted inventory: %+v", m.Folders[0].Subjects)
	}
}

func tableNames(t *testing.T, db *sql.DB) map[string]struct{} {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), "SELECT name FROM sqlite_master WHERE type = 'table'")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	out := make(map[string]struct{})
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		out[n] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func treeListing(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		b.WriteString(p)
		b.WriteString(":")
		b.WriteString(info.ModTime().String())
		b.WriteString(";")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}
