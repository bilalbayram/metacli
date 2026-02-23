package enterprise

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	approvalTokenVersion = 1

	approvalTokenTypeRequest = "approval_request"
	approvalTokenTypeGrant   = "approval_grant"

	ApprovalDecisionApproved = "approved"
	ApprovalDecisionRejected = "rejected"

	ApprovalStatusNotRequired         = "not_required"
	ApprovalStatusRequired            = "required"
	ApprovalStatusApproved            = "approved"
	ApprovalStatusRejected            = "rejected"
	ApprovalStatusExpired             = "expired"
	ApprovalStatusFingerprintMismatch = "fingerprint_mismatch"
	ApprovalStatusInvalid             = "invalid"
)

var highRiskCommandSet = map[string]struct{}{
	"auth add system-user": {},
	"auth page-token":      {},
	"auth app-token set":   {},
	"auth rotate":          {},
	"api post":             {},
	"api delete":           {},
	"schema sync":          {},
}

type ApprovalRequestTokenRequest struct {
	Principal     string
	Command       string
	OrgName       string
	WorkspaceName string
	TTL           time.Duration
	Now           func() time.Time
}

type ApprovalRequestTokenResponse struct {
	RequestToken      string `json:"request_token"`
	Fingerprint       string `json:"fingerprint"`
	Principal         string `json:"principal"`
	NormalizedCommand string `json:"normalized_command"`
	OrgName           string `json:"org_name"`
	WorkspaceName     string `json:"workspace_name"`
	ExpiresAt         string `json:"expires_at"`
}

type ApprovalGrantTokenRequest struct {
	RequestToken string
	Approver     string
	Decision     string
	TTL          time.Duration
	Now          func() time.Time
}

type ApprovalGrantTokenResponse struct {
	GrantToken        string `json:"grant_token"`
	Fingerprint       string `json:"fingerprint"`
	Decision          string `json:"decision"`
	Approver          string `json:"approver"`
	Principal         string `json:"principal"`
	NormalizedCommand string `json:"normalized_command"`
	OrgName           string `json:"org_name"`
	WorkspaceName     string `json:"workspace_name"`
	ExpiresAt         string `json:"expires_at"`
}

type ApprovalGrantValidationRequest struct {
	GrantToken    string
	Principal     string
	Command       string
	OrgName       string
	WorkspaceName string
	Now           func() time.Time
}

type ApprovalGrantValidationResult struct {
	Valid               bool   `json:"valid"`
	Status              string `json:"status"`
	DenyReason          string `json:"deny_reason,omitempty"`
	Fingerprint         string `json:"fingerprint,omitempty"`
	ExpectedFingerprint string `json:"expected_fingerprint,omitempty"`
	Decision            string `json:"decision,omitempty"`
	Approver            string `json:"approver,omitempty"`
	ExpiresAt           string `json:"expires_at,omitempty"`
}

type approvalGateRequest struct {
	Principal     string
	Command       string
	OrgName       string
	WorkspaceName string
	Token         string
	Now           func() time.Time
}

type approvalGateTrace struct {
	Required    bool
	Status      string
	DenyReason  string
	Fingerprint string
	Decision    string
	Approver    string
	ExpiresAt   string
}

type approvalRequestTokenClaims struct {
	Version          int    `json:"version"`
	TokenType        string `json:"token_type"`
	Principal        string `json:"principal"`
	Command          string `json:"command"`
	OrgName          string `json:"org_name"`
	WorkspaceName    string `json:"workspace_name"`
	Fingerprint      string `json:"fingerprint"`
	RequestedAt      string `json:"requested_at"`
	RequestExpiresAt string `json:"request_expires_at"`
}

type approvalGrantTokenClaims struct {
	Version        int    `json:"version"`
	TokenType      string `json:"token_type"`
	Principal      string `json:"principal"`
	Command        string `json:"command"`
	OrgName        string `json:"org_name"`
	WorkspaceName  string `json:"workspace_name"`
	Fingerprint    string `json:"fingerprint"`
	Decision       string `json:"decision"`
	Approver       string `json:"approver"`
	ApprovedAt     string `json:"approved_at"`
	GrantExpiresAt string `json:"grant_expires_at"`
}

func CreateApprovalRequestToken(request ApprovalRequestTokenRequest) (ApprovalRequestTokenResponse, error) {
	ttl := request.TTL
	if ttl <= 0 {
		return ApprovalRequestTokenResponse{}, errors.New("ttl must be greater than zero")
	}

	principal, normalizedCommand, orgName, workspaceName, err := normalizeApprovalContext(
		request.Principal,
		request.Command,
		request.OrgName,
		request.WorkspaceName,
	)
	if err != nil {
		return ApprovalRequestTokenResponse{}, err
	}

	now := approvalNow(request.Now)
	expiresAt := now.Add(ttl).UTC()
	fingerprint := approvalFingerprint(principal, normalizedCommand, orgName, workspaceName)
	claims := approvalRequestTokenClaims{
		Version:          approvalTokenVersion,
		TokenType:        approvalTokenTypeRequest,
		Principal:        principal,
		Command:          normalizedCommand,
		OrgName:          orgName,
		WorkspaceName:    workspaceName,
		Fingerprint:      fingerprint,
		RequestedAt:      now.UTC().Format(time.RFC3339Nano),
		RequestExpiresAt: expiresAt.Format(time.RFC3339Nano),
	}

	token, err := encodeApprovalToken(claims)
	if err != nil {
		return ApprovalRequestTokenResponse{}, err
	}
	return ApprovalRequestTokenResponse{
		RequestToken:      token,
		Fingerprint:       fingerprint,
		Principal:         principal,
		NormalizedCommand: normalizedCommand,
		OrgName:           orgName,
		WorkspaceName:     workspaceName,
		ExpiresAt:         expiresAt.Format(time.RFC3339Nano),
	}, nil
}

func CreateApprovalGrantToken(request ApprovalGrantTokenRequest) (ApprovalGrantTokenResponse, error) {
	ttl := request.TTL
	if ttl <= 0 {
		return ApprovalGrantTokenResponse{}, errors.New("ttl must be greater than zero")
	}
	approver := strings.TrimSpace(request.Approver)
	if approver == "" {
		return ApprovalGrantTokenResponse{}, errors.New("approver is required")
	}
	decision, err := normalizeApprovalDecision(request.Decision)
	if err != nil {
		return ApprovalGrantTokenResponse{}, err
	}

	requestClaims, err := decodeApprovalRequestToken(strings.TrimSpace(request.RequestToken))
	if err != nil {
		return ApprovalGrantTokenResponse{}, err
	}
	if requestClaims.TokenType != approvalTokenTypeRequest {
		return ApprovalGrantTokenResponse{}, fmt.Errorf("approval request token type %q is invalid", requestClaims.TokenType)
	}

	now := approvalNow(request.Now)
	requestExpiresAt, err := parseApprovalTimestamp(requestClaims.RequestExpiresAt, "request_expires_at")
	if err != nil {
		return ApprovalGrantTokenResponse{}, err
	}
	if !now.UTC().Before(requestExpiresAt) {
		return ApprovalGrantTokenResponse{}, fmt.Errorf("approval request token expired at %s", requestClaims.RequestExpiresAt)
	}

	principal, normalizedCommand, orgName, workspaceName, err := normalizeApprovalContext(
		requestClaims.Principal,
		requestClaims.Command,
		requestClaims.OrgName,
		requestClaims.WorkspaceName,
	)
	if err != nil {
		return ApprovalGrantTokenResponse{}, err
	}
	fingerprint := approvalFingerprint(principal, normalizedCommand, orgName, workspaceName)
	if requestClaims.Fingerprint != fingerprint {
		return ApprovalGrantTokenResponse{}, errors.New("approval request token fingerprint does not match token context")
	}

	expiresAt := now.Add(ttl).UTC()
	claims := approvalGrantTokenClaims{
		Version:        approvalTokenVersion,
		TokenType:      approvalTokenTypeGrant,
		Principal:      principal,
		Command:        normalizedCommand,
		OrgName:        orgName,
		WorkspaceName:  workspaceName,
		Fingerprint:    fingerprint,
		Decision:       decision,
		Approver:       approver,
		ApprovedAt:     now.UTC().Format(time.RFC3339Nano),
		GrantExpiresAt: expiresAt.Format(time.RFC3339Nano),
	}

	token, err := encodeApprovalToken(claims)
	if err != nil {
		return ApprovalGrantTokenResponse{}, err
	}
	return ApprovalGrantTokenResponse{
		GrantToken:        token,
		Fingerprint:       fingerprint,
		Decision:          decision,
		Approver:          approver,
		Principal:         principal,
		NormalizedCommand: normalizedCommand,
		OrgName:           orgName,
		WorkspaceName:     workspaceName,
		ExpiresAt:         expiresAt.Format(time.RFC3339Nano),
	}, nil
}

func ValidateApprovalGrantToken(request ApprovalGrantValidationRequest) (ApprovalGrantValidationResult, error) {
	principal, normalizedCommand, orgName, workspaceName, err := normalizeApprovalContext(
		request.Principal,
		request.Command,
		request.OrgName,
		request.WorkspaceName,
	)
	if err != nil {
		return ApprovalGrantValidationResult{}, err
	}

	expectedFingerprint := approvalFingerprint(principal, normalizedCommand, orgName, workspaceName)
	claims, err := decodeApprovalGrantToken(strings.TrimSpace(request.GrantToken))
	if err != nil {
		return ApprovalGrantValidationResult{}, err
	}
	if claims.TokenType != approvalTokenTypeGrant {
		return ApprovalGrantValidationResult{}, fmt.Errorf("approval grant token type %q is invalid", claims.TokenType)
	}

	claimPrincipal, claimCommand, claimOrg, claimWorkspace, err := normalizeApprovalContext(
		claims.Principal,
		claims.Command,
		claims.OrgName,
		claims.WorkspaceName,
	)
	if err != nil {
		return ApprovalGrantValidationResult{}, err
	}
	claimFingerprint := approvalFingerprint(claimPrincipal, claimCommand, claimOrg, claimWorkspace)
	if claims.Fingerprint != claimFingerprint {
		return ApprovalGrantValidationResult{}, errors.New("approval grant token fingerprint does not match token context")
	}

	decision, err := normalizeApprovalDecision(claims.Decision)
	if err != nil {
		return ApprovalGrantValidationResult{}, err
	}
	expiresAt, err := parseApprovalTimestamp(claims.GrantExpiresAt, "grant_expires_at")
	if err != nil {
		return ApprovalGrantValidationResult{}, err
	}

	result := ApprovalGrantValidationResult{
		Valid:               false,
		Status:              ApprovalStatusInvalid,
		Fingerprint:         claimFingerprint,
		ExpectedFingerprint: expectedFingerprint,
		Decision:            decision,
		Approver:            strings.TrimSpace(claims.Approver),
		ExpiresAt:           expiresAt.Format(time.RFC3339Nano),
	}

	if claimFingerprint != expectedFingerprint {
		result.Status = ApprovalStatusFingerprintMismatch
		result.DenyReason = "approval grant fingerprint does not match command context"
		return result, nil
	}

	now := approvalNow(request.Now).UTC()
	if !now.Before(expiresAt) {
		result.Status = ApprovalStatusExpired
		result.DenyReason = fmt.Sprintf("approval grant expired at %s", claims.GrantExpiresAt)
		return result, nil
	}

	switch decision {
	case ApprovalDecisionApproved:
		result.Valid = true
		result.Status = ApprovalStatusApproved
		return result, nil
	case ApprovalDecisionRejected:
		result.Status = ApprovalStatusRejected
		result.DenyReason = fmt.Sprintf("approval grant was rejected by %q", strings.TrimSpace(claims.Approver))
		return result, nil
	default:
		return ApprovalGrantValidationResult{}, fmt.Errorf("approval decision %q is not supported", claims.Decision)
	}
}

func evaluateApprovalGate(request approvalGateRequest) (approvalGateTrace, error) {
	if !requiresApprovalForCommand(request.Command) {
		return approvalGateTrace{
			Required: false,
			Status:   ApprovalStatusNotRequired,
		}, nil
	}

	normalizedPrincipal := strings.TrimSpace(request.Principal)
	normalizedOrg := strings.TrimSpace(request.OrgName)
	normalizedWorkspace := strings.TrimSpace(request.WorkspaceName)
	normalizedCommand := strings.TrimSpace(request.Command)
	fingerprint := approvalFingerprint(normalizedPrincipal, normalizedCommand, normalizedOrg, normalizedWorkspace)
	trace := approvalGateTrace{
		Required:    true,
		Status:      ApprovalStatusRequired,
		Fingerprint: fingerprint,
	}

	token := strings.TrimSpace(request.Token)
	if token == "" {
		trace.DenyReason = fmt.Sprintf("approval token is required for high-risk command %q", normalizedCommand)
		return trace, errors.New(trace.DenyReason)
	}

	validationResult, err := ValidateApprovalGrantToken(ApprovalGrantValidationRequest{
		GrantToken:    token,
		Principal:     normalizedPrincipal,
		Command:       normalizedCommand,
		OrgName:       normalizedOrg,
		WorkspaceName: normalizedWorkspace,
		Now:           request.Now,
	})
	if err != nil {
		trace.Status = ApprovalStatusInvalid
		trace.DenyReason = fmt.Sprintf("invalid approval token: %v", err)
		return trace, errors.New(trace.DenyReason)
	}

	trace.Status = validationResult.Status
	trace.Decision = validationResult.Decision
	trace.Approver = validationResult.Approver
	trace.ExpiresAt = validationResult.ExpiresAt
	trace.Fingerprint = validationResult.ExpectedFingerprint
	if !validationResult.Valid {
		trace.DenyReason = validationResult.DenyReason
		if trace.DenyReason == "" {
			trace.DenyReason = "approval grant denied"
		}
		return trace, errors.New(trace.DenyReason)
	}
	return trace, nil
}

func requiresApprovalForCommand(command string) bool {
	_, ok := highRiskCommandSet[strings.TrimSpace(command)]
	return ok
}

func normalizeApprovalContext(principal, command, orgName, workspaceName string) (string, string, string, string, error) {
	normalizedPrincipal := strings.TrimSpace(principal)
	if normalizedPrincipal == "" {
		return "", "", "", "", errors.New("principal is required")
	}
	normalizedCommand, err := normalizeCommandReference(command)
	if err != nil {
		return "", "", "", "", err
	}
	normalizedOrg := strings.TrimSpace(orgName)
	if normalizedOrg == "" {
		return "", "", "", "", errors.New("org_name is required")
	}
	normalizedWorkspace := strings.TrimSpace(workspaceName)
	if normalizedWorkspace == "" {
		return "", "", "", "", errors.New("workspace_name is required")
	}
	return normalizedPrincipal, normalizedCommand, normalizedOrg, normalizedWorkspace, nil
}

func approvalFingerprint(principal, normalizedCommand, orgName, workspaceName string) string {
	payload := strings.Join([]string{principal, normalizedCommand, orgName, workspaceName}, "\n")
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

func normalizeApprovalDecision(value string) (string, error) {
	decision := strings.ToLower(strings.TrimSpace(value))
	switch decision {
	case ApprovalDecisionApproved, ApprovalDecisionRejected:
		return decision, nil
	default:
		return "", fmt.Errorf("approval decision %q is not supported", value)
	}
}

func parseApprovalTimestamp(value, fieldName string) (time.Time, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return time.Time{}, fmt.Errorf("%s is required", fieldName)
	}
	timestamp, err := time.Parse(time.RFC3339Nano, normalized)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse %s: %w", fieldName, err)
	}
	return timestamp.UTC(), nil
}

func approvalNow(now func() time.Time) time.Time {
	if now == nil {
		return time.Now()
	}
	return now()
}

func encodeApprovalToken(claims any) (string, error) {
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("encode approval token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeApprovalRequestToken(token string) (approvalRequestTokenClaims, error) {
	claims := approvalRequestTokenClaims{}
	if err := decodeApprovalToken(token, &claims); err != nil {
		return approvalRequestTokenClaims{}, err
	}
	if claims.Version != approvalTokenVersion {
		return approvalRequestTokenClaims{}, fmt.Errorf("approval token version %d is unsupported", claims.Version)
	}
	return claims, nil
}

func decodeApprovalGrantToken(token string) (approvalGrantTokenClaims, error) {
	claims := approvalGrantTokenClaims{}
	if err := decodeApprovalToken(token, &claims); err != nil {
		return approvalGrantTokenClaims{}, err
	}
	if claims.Version != approvalTokenVersion {
		return approvalGrantTokenClaims{}, fmt.Errorf("approval token version %d is unsupported", claims.Version)
	}
	return claims, nil
}

func decodeApprovalToken(token string, claims any) error {
	if strings.TrimSpace(token) == "" {
		return errors.New("approval token is required")
	}
	payload, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return fmt.Errorf("decode approval token payload: %w", err)
	}
	if err := json.Unmarshal(payload, claims); err != nil {
		return fmt.Errorf("decode approval token claims: %w", err)
	}
	return nil
}
