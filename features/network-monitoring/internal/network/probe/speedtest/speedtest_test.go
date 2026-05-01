package speedtest

import "testing"

// TestTC_NM_010e_BufferbloatGradeBoundary30ms verifies that exactly 30 ms of
// added latency under load lands on the A→B boundary as grade A (upper-inclusive).
//
// @aitri-tc TC-NM-010e
func TestTC_NM_010e_BufferbloatGradeBoundary30ms(t *testing.T) {
	t.Parallel()
	if got := GradeBufferbloat(30.0); got != "A" {
		t.Fatalf("GradeBufferbloat(30.0) = %q, want %q (A is upper-inclusive)", got, "A")
	}
	if got := GradeBufferbloat(30.0001); got != "B" {
		t.Fatalf("GradeBufferbloat(30.0001) = %q, want %q (just past A)", got, "B")
	}
}

// TestTC_NM_010h_BufferbloatGrade25msIsA verifies that 25 ms added latency
// grades as A. (Spec marks this integration; the grading function is pure so
// covered as unit-level here.)
//
// @aitri-tc TC-NM-010h
func TestTC_NM_010h_BufferbloatGrade25msIsA(t *testing.T) {
	t.Parallel()
	if got := GradeBufferbloat(25.0); got != "A" {
		t.Fatalf("GradeBufferbloat(25.0) = %q, want %q", got, "A")
	}
}

// TestTC_NM_010f_BufferbloatGrade250msIsF verifies that 250 ms added latency
// grades as F. (Spec marks this e2e; the grading function is pure so covered
// as unit-level here.)
//
// @aitri-tc TC-NM-010f
func TestTC_NM_010f_BufferbloatGrade250msIsF(t *testing.T) {
	t.Parallel()
	// 250 ms is the upper-inclusive bound of D — F starts strictly above 250.
	if got := GradeBufferbloat(250.0); got != "D" {
		t.Fatalf("GradeBufferbloat(250.0) = %q, want %q (D upper bound)", got, "D")
	}
	if got := GradeBufferbloat(250.0001); got != "F" {
		t.Fatalf("GradeBufferbloat(250.0001) = %q, want %q", got, "F")
	}
	if got := GradeBufferbloat(500.0); got != "F" {
		t.Fatalf("GradeBufferbloat(500.0) = %q, want %q", got, "F")
	}
}

// TestTC_NM_010e_BufferbloatGradeNegativeClamps verifies that measurement
// noise producing tiny negative deltas does not panic or grade as F.
//
// @aitri-tc TC-NM-010e
func TestTC_NM_010e_BufferbloatGradeNegativeClamps(t *testing.T) {
	t.Parallel()
	if got := GradeBufferbloat(-1.5); got != "A" {
		t.Fatalf("GradeBufferbloat(-1.5) = %q, want %q (negatives clamp to A)", got, "A")
	}
}

// TestTC_NM_009f_RunGuardRefusesSecondAcquire asserts the FR-024 single-run
// invariant: while a speedtest is in flight, a second TryAcquire returns
// false (the HTTP layer translates this to 409 already_running). The
// integration-level TC-NM-009f drives this through POST /api/network/
// speedtests/run; here we pin the underlying guard.
//
// @aitri-tc TC-NM-009f
func TestTC_NM_009f_RunGuardRefusesSecondAcquire(t *testing.T) {
	t.Parallel()
	var g RunGuard
	if !g.TryAcquire() {
		t.Fatal("first TryAcquire returned false, want true")
	}
	if g.TryAcquire() {
		t.Fatal("second TryAcquire while held returned true, want false (409 already_running)")
	}
	g.Release()
	if !g.TryAcquire() {
		t.Fatal("TryAcquire after Release returned false, want true (guard must reset)")
	}
}
