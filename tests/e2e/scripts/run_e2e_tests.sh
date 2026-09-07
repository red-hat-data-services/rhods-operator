#!/usr/bin/env bash
set -eo pipefail

# Run the RHOAI/ODH operator e2e suite and produce a leaf-oriented JUnit report.
#
# Go reports a failed subtest and every failed parent as separate failures.
# Keep the raw artifacts for diagnostics, then exclude parent results from the
# JUnit report consumed by the CI failure importer, so deeply nested subtest
# failures produce one actionable failure instead of one failure per ancestor.

mkdir -p results

echo "Using gotestsum with leaf-oriented JUnit XML"

# Keep raw JUnit content outside *.xml globs so CI ingests only the final report.
raw_junit_report=results/xunit_report.unfiltered
test_events=results/test-events.json

set +e
gotestsum --junitfile-project-name odh-operator-e2e \
  --junitfile "$raw_junit_report" --jsonfile "$test_events" \
  --format testname --raw-command \
  -- test2json -p e2e ./e2e-tests --test.parallel=1 --test.v=test2json --deletion-policy=never \
  --operator-namespace="$E2E_TEST_OPERATOR_NAMESPACE" \
  --applications-namespace="$E2E_TEST_APPLICATIONS_NAMESPACE" \
  --workbenches-namespace="$E2E_TEST_WORKBENCHES_NAMESPACE" \
  --dsc-monitoring-namespace="$E2E_TEST_DSC_MONITORING_NAMESPACE" \
  "$@"
test_status=$?
set -e

# Ignore only parent results. Their captured output is retained by the
# converter as suite-level output. If this leaves no failure/error despite
# a failed test process, retain the unfiltered report so parent-only
# failures (for example setup, cleanup, or watchdog failures) remain visible.
if go-junit-report -parser gojson \
  -subtest-mode ignore-parent-results \
  -in "$test_events" \
  -out results/xunit_report.xml; then
  if [ "$test_status" -ne 0 ] && \
    ! grep -Eq '<(failure|error)([[:space:]>])' results/xunit_report.xml; then
    echo "Warning: no leaf failure found; retaining unfiltered JUnit report" >&2
    cp -- "$raw_junit_report" results/xunit_report.xml
  fi
else
  report_status=$?
  echo "Error: failed to generate leaf-oriented JUnit report" >&2
  if [ -s "$raw_junit_report" ]; then
    echo "Retaining unfiltered JUnit report"
    cp -- "$raw_junit_report" results/xunit_report.xml
  else
    if [ "$test_status" -ne 0 ]; then
      exit "$test_status"
    fi
    exit "$report_status"
  fi
fi

exit "$test_status"
