package ingest

import (
	"context"
	"net/http"
	"strings"

	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

// Routing hint headers. They are assertions, never the source of truth: the
// token resolves the tenant/project and a mismatch is rejected with 403
// (AD-3, Story 4.2 AC2).
const (
	HeaderTenantID  = "X-Observaki-Tenant-Id"
	HeaderProjectID = "X-Observaki-Project-Id"
)

func (g *Gateway) authenticateHTTP(r *http.Request, signal Signal, requestID string) (Identity, *requestError) {
	if g.auth == nil {
		return Identity{}, nil
	}
	identity, requestErr := g.auth.Authenticate(requestID, r.Header.Get("Authorization"))
	if requestErr != nil {
		g.observeAuthError(identity, signal, protocolHTTP, r.RemoteAddr, requestID, requestErr)
		return identity, requestErr
	}
	requestErr = g.checkRoutingHints(
		identity,
		signal,
		protocolHTTP,
		r.RemoteAddr,
		requestID,
		r.Header.Get(HeaderTenantID),
		r.Header.Get(HeaderProjectID),
	)
	if requestErr != nil {
		g.observeAuthError(identity, signal, protocolHTTP, r.RemoteAddr, requestID, requestErr)
		return identity, requestErr
	}
	return identity, nil
}

func (g *Gateway) authenticateGRPC(ctx context.Context, signal Signal, requestID string) (Identity, *requestError) {
	if g.auth == nil {
		return Identity{}, nil
	}
	identity, requestErr := g.auth.Authenticate(requestID, metadataValue(ctx, "authorization"))
	if requestErr != nil {
		g.observeAuthError(identity, signal, protocolGRPC, remoteAddr(ctx), requestID, requestErr)
		return identity, requestErr
	}
	requestErr = g.checkRoutingHints(
		identity,
		signal,
		protocolGRPC,
		remoteAddr(ctx),
		requestID,
		metadataValue(ctx, strings.ToLower(HeaderTenantID)),
		metadataValue(ctx, strings.ToLower(HeaderProjectID)),
	)
	if requestErr != nil {
		g.observeAuthError(identity, signal, protocolGRPC, remoteAddr(ctx), requestID, requestErr)
		return identity, requestErr
	}
	return identity, nil
}

func (g *Gateway) checkRoutingHints(
	identity Identity,
	signal Signal,
	protocol protocol,
	remoteAddr, requestID, tenantHint, projectHint string,
) *requestError {
	tenantHint = strings.TrimSpace(tenantHint)
	projectHint = strings.TrimSpace(projectHint)
	tenantMismatch := tenantHint != "" && tenantHint != identity.TenantID
	projectMismatch := projectHint != "" && projectHint != identity.ProjectID
	if !tenantMismatch && !projectMismatch {
		return nil
	}
	g.audit(AuditEvent{
		ActorType:       AuditActorToken,
		ActorID:         identity.TokenID,
		Action:          AuditActionCrossProjectWrite,
		Outcome:         AuditOutcomeDenied,
		TenantID:        identity.TenantID,
		ProjectID:       identity.ProjectID,
		TargetTenantID:  tenantHint,
		TargetProjectID: projectHint,
		RequestID:       requestID,
		Signal:          signal.String(),
		Protocol:        protocol.String(),
		RemoteAddr:      remoteAddr,
		Reason:          "cross_project_write",
	})
	return errCrossProjectWrite(requestID)
}

func (g *Gateway) observeAuthError(
	identity Identity,
	signal Signal,
	protocol protocol,
	remoteAddr, requestID string,
	requestErr *requestError,
) {
	g.metrics.observeAuthFailure(requestErr.reason)
	if requestErr.reason != "insufficient_scope" {
		return
	}
	g.audit(AuditEvent{
		ActorType:  AuditActorToken,
		ActorID:    identity.TokenID,
		Action:     AuditActionInsufficientScope,
		Outcome:    AuditOutcomeDenied,
		TenantID:   identity.TenantID,
		ProjectID:  identity.ProjectID,
		RequestID:  requestID,
		Signal:     signal.String(),
		Protocol:   protocol.String(),
		RemoteAddr: remoteAddr,
		Reason:     "insufficient_scope",
	})
}

func (g *Gateway) checkQuota(requestID string, identity Identity, events int) *requestError {
	if g.auth == nil || events <= 0 {
		return nil
	}
	_, requestErr := g.auth.Admit(requestID, identity, events)
	if requestErr == nil {
		return nil
	}
	if requestErr.reason == "quota_exceeded" {
		g.metrics.observeQuotaRejection(events)
	} else {
		g.metrics.observeAuthFailure(requestErr.reason)
	}
	return requestErr
}

func (g *Gateway) reserveHTTP(r *http.Request, identity Identity, requestID string) (*Reservation, *requestError) {
	if g.auth == nil {
		return nil, nil
	}
	reservation, requestErr := g.auth.Reserve(requestID, identity, r.ContentLength)
	if requestErr == nil {
		return reservation, nil
	}
	if requestErr.reason == "quota_exceeded" {
		g.metrics.observeQuotaRejection(0)
	} else {
		g.metrics.observeAuthFailure(requestErr.reason)
	}
	return nil, requestErr
}

func (g *Gateway) settleHTTP(requestID string, reservation *Reservation, events int) *requestError {
	if g.auth == nil {
		return nil
	}
	requestErr := g.auth.Settle(requestID, reservation, events)
	if requestErr == nil {
		return nil
	}
	if requestErr.reason == "quota_exceeded" {
		g.metrics.observeQuotaRejection(events)
	} else {
		g.metrics.observeAuthFailure(requestErr.reason)
	}
	return requestErr
}

func (g *Gateway) audit(event AuditEvent) {
	if g.auth == nil {
		return
	}
	g.auth.Audit(event)
}

func metadataValue(ctx context.Context, key string) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	values := md.Get(key)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func remoteAddr(ctx context.Context) string {
	if info, ok := peer.FromContext(ctx); ok && info.Addr != nil {
		return info.Addr.String()
	}
	return ""
}
