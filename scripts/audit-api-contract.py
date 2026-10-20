#!/usr/bin/env python3
"""Bounded real-HTTP response audit. Requires PyYAML and jsonschema >= 4.10.

No network resolution; only checked-in OpenAPI files. Raw synthetic responses
(including test credentials) live only in a private, automatically removed temp
directory. The output is a body-free operation/status/test matrix.
"""
import argparse
import base64
import csv
from copy import deepcopy
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
from importlib.metadata import version
from urllib.parse import unquote, urlsplit

import yaml
from jsonschema import Draft202012Validator, FormatChecker

ROOT = Path(__file__).resolve().parents[1]
SPEC = ROOT / 'api/openapi/openapi.yaml'
METHODS = {'get', 'put', 'post', 'delete', 'options', 'head', 'patch', 'trace'}
# Existing consumer tests retain their independent status, financial, mutation,
# pagination and binary-content assertions. Capture adds schema validation.
TESTS = [
 'TestSetupStatusReturnsInitialSteps',
 'TestCreateOwnerRequiresConfiguredSetupToken',
 'TestAuthSessionReturnsUnauthenticatedWithoutCookie',
 'TestAuthSessionReturnsAuthenticatedUserForValidCookie',
 'TestLoginRejectsInvalidCredentials',
 'TestLoginRejectsNonJSONContentType',
 'TestLogoutRejectsInvalidCSRFToken',
 'TestOversizedJSONRequestUsesDocumentedError',
 'TestReferenceBookProducesItsRegister',
 'TestTransactionValidationAndLifecycleGuards',
 'TestListDeletedTransactionsCursorPagination',
 'TestUnknownProjectedBasisAPIKeepsQuantityAndReturnsNullBasis',
 'TestInvestmentMoneyAboveJavaScriptSafeIntegerRoundTripsAsStrings',
 'TestPriceCoefficientsRemainExactAboveBrowserSafeInteger',
 'TestPricingSourceAssignments_CreateUpdateList_HTTP',
 'TestRecurringTemplateAPICreatePatchReadArchive',
 'TestForecastAPIReturnsExactWireQuantities',
 'TestBudgetTargetRejectsNonMonthBoundary',
 'TestStandaloneLedgerCSVRejectsScopeParameters',
 'TestLedgerExportRendersStoredScaleWithoutFloat',
 'TestQIFRoundTripsUnderTheDeclaredLayout',
 'TestImportUploadFileSizeBoundary',
 'TestImportUploadMultipartResourceLimits',
 'TestPostingResponseContractEmptyAndNonemptyTags',
 'TestSpendingResponseContractNamesActualExcludedSystemRoles',
 'TestBackupStatusServesDefaultsAndTheKeyNotice',
 'TestBackupPolicyUpdateKeepsOmittedFields',
 'TestRequestBackupReportsAcceptedWorkNotACompletedBackup',
 'TestExportBundleChecksumsVerify',
 'TestInvestmentGainsRunReproducesBeforeSaleReversal',
 'TestInvestmentGainsRunCursorPagination',
 'TestMutationResponseEnrichmentBuyMatchesCanonicalRead',
 'TestMutationResponseEnrichmentReconciliationBatchMatchesCanonicalReads',
 'TestMutationResponseEnrichmentFailurePreservesCommittedBuyWithoutRetry',
 'TestMutationResponseEnrichmentFailurePreservesCompoundCorrection',
 'TestMutationResponseEnrichmentExternalOutReplacementAndReversal',
 'TestReverseManualSaleAPI',
 'TestReverseManualBuyAPI',
 'TestReplaceLatestManualSaleAPI',
 'TestReplaceOldManualBuyAPI',
 'TestShortSaleAndCoverAPI',
 'TestShortCorrectionAPI',
 'TestCreateReinvestedDividend_HTTP',
 'TestReplaceAndReverseReinvestedDividendAPI',
 'TestReplaceAndReverseDividendAPI',
 'TestPaymentInLieuEntryAndCorrectionAPI',
 'TestReplaceAndReverseWriteOffAPI',
 'TestCashInLieuAPICorrectReverseAndReleaseSplitFence',
 'TestSeparateWithholdingAPIAndBundle',
 'TestReverseInternalTransferAPI',
 'TestReplaceInternalTransferAPIPreviewMatchesCommit',
 'TestReplaceExternalTransferInAPI',
 'TestExternalTransferOutAPIPreviewMatchesCommitAndLabelsBridge',
 'TestResolveTransferBasisAPI',
 'TestCorrectTransferBasisResolutionAPI',
 'TestKnownZeroBasisResolutionAPI',
 'TestInvestmentSplitCorrectionAPIReplaceReverseAndLabels',
 'TestJournalFreeSplitOperationAPIAndActivityPagination',
 'TestCapitalReturnReplacementAPIPreviewMatchesCommitAndFencesKind',
 'TestShareExchangeCorrectionAPI',
 'TestSpinOffCorrectionAPI',
 'TestCashStockMergerCorrectionAPI',
]


def load_contract():
    documents = {}

    def resolve(node, source):
        if isinstance(node, list):
            return [resolve(value, source) for value in node]
        if not isinstance(node, dict):
            return node
        if '$ref' in node:
            reference = urlsplit(node['$ref'])
            if reference.scheme or reference.netloc or reference.query:
                raise ValueError('Only local contract references are supported')
            target = (source.parent / unquote(reference.path)).resolve() if reference.path else source
            if not target.is_relative_to(SPEC.parent):
                raise ValueError('Reference escapes the checked-in OpenAPI tree')
            if target not in documents:
                documents[target] = yaml.safe_load(target.read_text())
            value = documents[target]
            for part in unquote(reference.fragment).split('/')[1:]:
                value = value[part.replace('~1', '/').replace('~0', '~')]
            # This contract has no recursive schemas. Preserve 3.1 ref siblings.
            resolved = resolve(value, target)
            siblings = {key: resolve(value, source) for key, value in node.items() if key != '$ref'}
            if set(siblings) <= {'description', 'summary'}:
                return dict(resolved, **siblings)
            return {'allOf': [resolved, siblings]}
        return {key: resolve(value, source) for key, value in node.items()}

    return resolve(yaml.safe_load(SPEC.read_text()), SPEC)


VALIDATORS = {}


def validator(schema):
    # Validators retain their schema, so identity cannot be reused while cached.
    # Check each resolved operation schema once, rather than for every sample.
    if id(schema) not in VALIDATORS:
        Draft202012Validator.check_schema(schema)
        VALIDATORS[id(schema)] = Draft202012Validator(schema, format_checker=FormatChecker())
    return VALIDATORS[id(schema)]


def negative_controls(contract):
    """Independent payloads plus deliberately damaged resolved schemas.

    These are validator sensitivity controls, not invented HTTP evidence.
    Each baseline must validate before its paired negative is meaningful.
    """
    schemas = contract['components']['schemas']
    controls = []

    def check(name, schema, good, bad, damaged):
        if not validator(schema).is_valid(good):
            raise AssertionError(name + ': baseline rejected')
        if validator(schema).is_valid(bad):
            raise AssertionError(name + ': bad payload accepted')
        if validator(damaged).is_valid(good):
            raise AssertionError(name + ': schema mutation undetected')
        controls.append((name, 'valid baseline accepted; payload and schema mutations rejected'))

    error = schemas['ErrorResponse']
    good = {'error': {'code': 'VALIDATION_FAILED', 'message': 'synthetic control'}}
    damaged = deepcopy(error)
    damaged['required'].append('invented_required')
    check('required_error_envelope', error, good, {}, damaged)
    damaged = deepcopy(error)
    damaged['properties']['error']['properties']['code']['enum'] = ['INVENTED']
    check('error_code_enum', error, good,
          {'error': {'code': 'INVENTED', 'message': 'synthetic control'}}, damaged)

    # Extract actual contract fields: no copied numeric or nullable schema.
    posting = schemas['PostingResponse']['properties']
    coefficient = posting['quantity_value']
    damaged = deepcopy(coefficient)
    damaged['type'] = 'integer'
    check('exact_coefficient_string', coefficient, '9007199254740993', 9007199254740993, damaged)
    damaged = deepcopy(coefficient)
    damaged['pattern'] = '^0$'
    check('canonical_coefficient_pattern', coefficient, '-123', '01', damaged)
    nullable = posting['account_code']
    damaged = deepcopy(nullable)
    damaged['type'] = 'string'
    check('explicit_null', nullable, None, 42, damaged)
    nonnullable = schemas['ErrorBody']['properties']['message']
    damaged = deepcopy(nonnullable)
    damaged['type'] = 'null'
    check('nonnullable_message', nonnullable, 'synthetic control', None, damaged)

    # Omittable PATCH fields and explicit null must remain distinct. Use the
    # recurring request exercised by the real rename/clear HTTP consumer.
    patch = contract['paths']['/api/v1/recurring/templates/{template_id}']['patch']['requestBody']['content']['application/json']['schema']
    damaged = deepcopy(patch)
    damaged['required'] = ['enabled']
    check('patch_omission', patch, {'name': 'synthetic control'}, {'enabled': 'false'}, damaged)
    damaged = deepcopy(patch)
    damaged['properties']['payee_id'] = {'type': 'integer'}
    check('patch_explicit_null', patch, {'payee_id': None}, {'payee_id': 'invalid'}, damaged)
    return controls


def validate_record(operation, record):
    status = str(record['status'])
    response = operation['responses'].get(status, operation['responses'].get('default'))
    if response is None:
        return ['undocumented status ' + status]
    body = base64.b64decode(record['body'] or '')
    content = response.get('content', {})
    if not content:
        return ['unexpected response body'] if body else []
    media = record['content_type'].split(';')[0].lower()
    if media not in content:
        return ['undocumented content type ' + media]
    schema = content[media]['schema']
    if media == 'application/json':
        try:
            instance = json.loads(body)
        except (ValueError, UnicodeError):
            return ['invalid JSON']
        # Never include actual values in evidence (tokens, CSRF, etc.).
        def locations(error):
            # For exactly object-or-null, expose the object's leaf failures
            # instead of allowing an opaque oneOf failure to hide new defects.
            if error.validator == 'oneOf' and isinstance(error.instance, dict):
                branches = error.validator_value
                if len(branches) == 2 and sorted(str(branch.get('type')) for branch in branches) == ['null', 'object']:
                    index = next(i for i, branch in enumerate(branches) if branch.get('type') == 'object')
                    return [location for child in error.context if child.schema_path[0] == index
                            for location in locations(child)]
            return ['/' + '/'.join(map(str, error.absolute_path)) + ': ' + error.validator]
        return sorted({location for error in validator(schema).iter_errors(instance)
                       for location in locations(error)})
    if schema.get('type') != 'string' or schema.get('format') != 'binary':
        return ['non-JSON media does not declare binary string']
    return [] if body else ['empty download']


def known_issue(key, status, error):
    """Narrow retained findings, never general operation-level exclusions."""
    if key == ('POST', '/api/v1/setup/owner') and status == 413 and error == 'undocumented status 413':
        return '#350'
    return ''


def summarize(operation, record):
    response = operation['responses'].get(str(record['status']), {})
    schemas = response.get('content', {})
    expected = ' | '.join(media + '; required=' + ','.join(value['schema'].get('required', []))
                          for media, value in schemas.items()) or 'no declared content'
    raw = base64.b64decode(record['body'] or '')
    if record['content_type'].split(';')[0] == 'application/json':
        try:
            body = json.loads(raw)
        except (ValueError, UnicodeError):
            return expected, 'invalid JSON'
        # Root field names and JSON types only; no financial or credential values.
        def kind(value):
            if value is None:
                return 'null'
            if isinstance(value, bool):
                return 'boolean'
            if isinstance(value, str):
                return 'string'
            if isinstance(value, dict):
                return 'object'
            if isinstance(value, list):
                return 'array'
            return 'number'
        actual = (','.join(key + ':' + kind(value) for key, value in sorted(body.items()))
                  if isinstance(body, dict) else kind(body))
    else:
        actual = str(len(raw)) + ' bytes'
    return expected, actual


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--strict', action='store_true', help='Fail for retained findings as well as new discrepancies')
    parser.add_argument('--controls-only', action='store_true', help='Run schema sensitivity controls without HTTP fixtures')
    args = parser.parse_args()
    contract = load_contract()
    controls = negative_controls(contract)
    args.output.mkdir(parents=True, exist_ok=True)
    with (args.output / 'controls.csv').open('w') as file:
        writer = csv.writer(file)
        writer.writerow(['control', 'actual'])
        writer.writerows(controls)
    if args.controls_only:
        print(str(len(controls)) + ' schema sensitivity controls passed')
        return 0
    operations = {(method.upper(), path): operation
                  for path, item in contract['paths'].items()
                  for method, operation in item.items() if method in METHODS}
    patterns = [(method, path, re.compile('^' + re.sub(r'\{[^}]+\}', '[^/]+', path) + '$'))
                for method, path in sorted(operations, key=lambda key: (key[1].count('{'), key))]
    cases = {}
    summaries = {}
    with tempfile.TemporaryDirectory(prefix='rr-contract-audit-') as directory:
        environment = dict(os.environ, REKENRAAM_CONTRACT_AUDIT_DIR=directory)
        command = ['./scripts/go-backend.sh', 'test', './internal/api', '-count=1',
                   '-timeout=5m', '-run', '^(' + '|'.join(TESTS) + ')$']
        subprocess.run(command, cwd=ROOT, env=environment, check=True)
        for file in Path(directory).glob('response-*.json'):
            record = json.loads(file.read_text())
            key = next(((method, path) for method, path, pattern in patterns
                        if method == record['method'] and pattern.fullmatch(record['path'])), None)
            if key is None:
                raise ValueError('Captured request has no documented operation: ' + record['method'] + ' ' + record['path'])
            errors = validate_record(operations[key], record)
            if record['status'] >= 400:
                # Even an undocumented status must carry a conforming error
                # envelope; a retained status gap cannot hide new JSON drift.
                envelope_operation = {'responses': {str(record['status']): {'content': {
                    'application/json': {'schema': contract['components']['schemas']['ErrorResponse']}}}}}
                errors = sorted(set(errors + validate_record(envelope_operation, record)))
            case = (*key, str(record['status']), record['content_type'], record['test'], '; '.join(errors))
            cases[case] = cases.get(case, 0) + 1
            summaries[case] = summarize(operations[key], record)
    with (args.output / 'responses.csv').open('w') as file:
        writer = csv.writer(file)
        writer.writerow(['method', 'path', 'status', 'actual_content_type', 'test', 'schema_errors',
                         'samples', 'contract_expectation', 'actual_response_shape', 'disposition'])
        for case, count in sorted(cases.items()):
            errors = case[-1].split('; ') if case[-1] else []
            issues = {known_issue(case[:2], int(case[2]), error) or 'NEW' for error in errors}
            writer.writerow([*case, count, *summaries[case], ','.join(sorted(issues)) or 'conformant sample'])
    with (args.output / 'operations.csv').open('w') as file:
        writer = csv.writer(file)
        writer.writerow(['method', 'path', 'declared_statuses', 'observed_statuses', 'unverified_declared_statuses', 'disposition'])
        for key, operation in sorted(operations.items()):
            observed = [case for case in cases if case[:2] == key]
            writer.writerow([*key, ' '.join(sorted(operation['responses'])),
                             ' '.join(sorted({case[2] for case in observed})),
                             ' '.join(sorted(set(operation['responses']) - {case[2] for case in observed})),
                             ('discrepancy' if any(case[-1] for case in observed) else 'sampled conformant')
                             if observed else 'unverified'])
    missing = set(TESTS) - {case[4].split('/')[0] for case in cases}
    if missing:
        raise ValueError('Selected tests produced no captures: ' + ', '.join(sorted(missing)))
    failures = sum(bool(case[-1]) for case in cases)
    unexpected = sum(any(not known_issue(case[:2], int(case[2]), error) for error in case[-1].split('; '))
                     for case in cases if case[-1])
    metadata = {'audited_sha': subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip(),
                'working_tree_changes': bool(subprocess.check_output(['git', 'status', '--porcelain'], cwd=ROOT)),
                'openapi': contract['openapi'], 'jsonschema': version('jsonschema'), 'PyYAML': version('PyYAML'),
                'asserted_formats': sorted(FormatChecker.checkers),
                'operations': len(operations), 'sampled_operations': len({case[:2] for case in cases}),
                      'operation_status_test_rows': len(cases), 'responses': sum(cases.values()),
                'discrepant_rows': failures, 'unexpected_discrepant_rows': unexpected,
                'schema_controls': len(controls), 'selected_tests': TESTS}
    (args.output / 'summary.json').write_text(json.dumps(metadata, indent=2) + '\n')
    print(json.dumps({key: value for key, value in metadata.items() if key != 'selected_tests'}, indent=2))
    return bool(failures if args.strict else unexpected)


if __name__ == '__main__':
    raise SystemExit(main())
