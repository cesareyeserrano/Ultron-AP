# Test Cases: pironman-optional-integration

### TC-1
- Title: Feature flag disabled path
- Trace: US-1, FR-1, AC-2
- Steps:
1. Given `ULTRON_FEATURE_PIRONMAN=false`
2. When requesting optional Pironman route
3. Then optional module is not exposed in core workflow

### TC-2
- Title: Feature flag enabled route
- Trace: US-1, FR-1, AC-2
- Steps:
1. Given `ULTRON_FEATURE_PIRONMAN=true`
2. When requesting optional Pironman page authenticated
3. Then page renders with integration capability section

### TC-3
- Title: Apply endpoint CSRF/session guard
- Trace: US-2, FR-2, FR-4, AC-1, AC-4
- Steps:
1. Given enabled feature and authenticated session
2. When apply request has invalid CSRF
3. Then response is forbidden

### TC-4
- Title: Input sanitization
- Trace: US-4, FR-4, AC-4
- Steps:
1. Given malformed payload values
2. When apply payload is parsed
3. Then values are normalized to allowed ranges and sets

### TC-5
- Title: Timeout capability mapping
- Trace: US-3, FR-3, AC-3
- Steps:
1. Given Pironman API timeout
2. When capability is computed
3. Then status is `degraded`

### TC-6
- Title: Socket unavailable mapping
- Trace: US-3, FR-3, AC-3
- Steps:
1. Given helper socket unavailable
2. When capability is computed
3. Then status is `unavailable`
