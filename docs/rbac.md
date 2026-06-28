# Role-Based Access Control (RBAC)

**Status**: Design Document  
**Target Phase**: 6b  
**Last Updated**: 2026-06-27

---

## Overview

This document defines StratumMC's authentication and authorization system. The current shared-token authentication model is replaced with a user account system, project membership, and role-based permissions.

**Design Principles**:
1. **Project-centric**: All permissions derive from Project membership. No global admin role except system bootstrap.
2. **Explicit over implicit**: Every operation checks permissions. No "if you can read, you can write" assumptions.
3. **Audit by default**: Every permission check failure creates an audit event.
4. **Least privilege**: Default deny. Roles grant explicit capabilities.
5. **Immutable membership history**: Membership changes are audited but never deleted.

---

## Terminology

| Term | Definition |
|------|------------|
| **User** | An individual with login credentials. Identified by unique UserID. |
| **Actor** | The UserID performing an action. Used in audit events and permission checks. |
| **Project Membership** | The relationship between a User and a Project, carrying a Role. |
| **Role** | A named set of capabilities within a Project. |
| **Permission** | A specific action on a specific resource type (e.g., "start session", "approve artifact"). |
| **System User** | Special actor for automated operations (e.g., agent heartbeat, scheduler). Not a real user. |

---

## Roles

StratumMC defines **four project-level roles** with strict hierarchy:

```text
Owner > Maintainer > Researcher > Viewer
```

Higher roles inherit all permissions from lower roles.

### Role Definitions

#### **Viewer** (Read-Only)
- **Purpose**: Observers, guests, trainees waiting for full access.
- **Can**:
  - List projects they are members of
  - View project/room/session metadata
  - View checkpoints and artifacts (metadata only)
  - View audit logs (for sessions they can see)
  - Inspect session logs and runtime observations
- **Cannot**:
  - Create, start, stop, or modify any resources
  - Upload or approve artifacts
  - Create checkpoints
  - Fork sessions

#### **Researcher** (Default Collaborator)
- **Purpose**: Active technical testers doing experiments.
- **Inherits**: All Viewer permissions
- **Additionally Can**:
  - Create fork sessions (subject to resource policy)
  - Create private sessions (subject to resource policy)
  - Create review sessions (subject to resource policy)
  - Start/stop/restart own sessions (fork/private/review)
  - Create checkpoints from any session
  - Restore checkpoints to own sessions only
  - Upload artifacts (but cannot approve them)
  - Apply approved artifacts to own sessions
  - Send commands to own sessions
- **Cannot**:
  - Modify shared sessions (start/stop/restart/checkpoint)
  - Approve artifacts
  - Modify project/room configuration
  - Add/remove project members

#### **Maintainer** (Session Manager)
- **Purpose**: Trusted collaborators managing shared infrastructure.
- **Inherits**: All Researcher permissions
- **Additionally Can**:
  - Create shared sessions
  - Start/stop/restart shared sessions
  - Create checkpoints from shared sessions
  - Restore checkpoints to shared sessions
  - Apply approved artifacts to shared sessions
  - Send commands to shared sessions
  - Approve artifacts for project use
  - Modify room configuration (default environment, base artifacts)
  - Create/modify environments scoped to the project
- **Cannot**:
  - Delete shared sessions (Owner only)
  - Modify project metadata (name, description)
  - Add/remove project members
  - Delete the project

#### **Owner** (Project Administrator)
- **Purpose**: Project creator and final authority.
- **Inherits**: All Maintainer permissions
- **Additionally Can**:
  - Modify project metadata (name, description, resource policies)
  - Add/remove project members
  - Change member roles (including promoting to Owner)
  - Delete shared sessions
  - Delete the project (if no active sessions)
  - Transfer project ownership
- **Cannot**:
  - Access other projects without being a member
  - Override system-level resource policies (set by deployment admin)

---

## Permission Matrix

| Action | Viewer | Researcher | Maintainer | Owner |
|--------|--------|------------|------------|-------|
| **Project** |
| View project metadata | ✅ | ✅ | ✅ | ✅ |
| Modify project metadata | ❌ | ❌ | ❌ | ✅ |
| Delete project | ❌ | ❌ | ❌ | ✅ |
| Add/remove members | ❌ | ❌ | ❌ | ✅ |
| Change member roles | ❌ | ❌ | ❌ | ✅ |
| **Room** |
| View room metadata | ✅ | ✅ | ✅ | ✅ |
| Modify room configuration | ❌ | ❌ | ✅ | ✅ |
| Delete room | ❌ | ❌ | ❌ | ✅ |
| **Session (Shared)** |
| View session metadata | ✅ | ✅ | ✅ | ✅ |
| View session logs | ✅ | ✅ | ✅ | ✅ |
| Create shared session | ❌ | ❌ | ✅ | ✅ |
| Start/stop/restart | ❌ | ❌ | ✅ | ✅ |
| Send commands | ❌ | ❌ | ✅ | ✅ |
| Delete shared session | ❌ | ❌ | ❌ | ✅ |
| **Session (Fork/Private/Review)** |
| Create own session | ❌ | ✅ | ✅ | ✅ |
| Start/stop/restart own | ❌ | ✅ | ✅ | ✅ |
| Send commands to own | ❌ | ✅ | ✅ | ✅ |
| Delete own session | ❌ | ✅ | ✅ | ✅ |
| **Checkpoint** |
| View checkpoint metadata | ✅ | ✅ | ✅ | ✅ |
| Create checkpoint (any session) | ❌ | ✅ | ✅ | ✅ |
| Restore to own session | ❌ | ✅ | ✅ | ✅ |
| Restore to shared session | ❌ | ❌ | ✅ | ✅ |
| Delete checkpoint | ❌ | ❌ | ❌ | ✅ |
| **Artifact** |
| View artifact metadata | ✅ | ✅ | ✅ | ✅ |
| Upload artifact | ❌ | ✅ | ✅ | ✅ |
| Approve artifact | ❌ | ❌ | ✅ | ✅ |
| Reject artifact | ❌ | ❌ | ✅ | ✅ |
| Apply to own session | ❌ | ✅ | ✅ | ✅ |
| Apply to shared session | ❌ | ❌ | ✅ | ✅ |
| Delete artifact | ❌ | ❌ | ❌ | ✅ |
| **Environment** |
| View environment | ✅ | ✅ | ✅ | ✅ |
| Create/modify project env | ❌ | ❌ | ✅ | ✅ |
| Delete environment | ❌ | ❌ | ❌ | ✅ |
| **Audit** |
| View audit events | ✅ | ✅ | ✅ | ✅ |

---

## Domain Models

### User

New domain model representing an authenticated individual.

```go
package user

import "time"

type User struct {
    ID           string    `json:"id"`
    Username     string    `json:"username"`          // Unique login name
    Email        string    `json:"email,omitempty"`   // Optional contact
    DisplayName  string    `json:"displayName"`       // Human-readable name
    PasswordHash string    `json:"-"`                 // Never serialized
    IsActive     bool      `json:"isActive"`          // Can be deactivated
    CreatedAt    time.Time `json:"createdAt"`
    UpdatedAt    time.Time `json:"updatedAt"`
}
```

**Validation Rules**:
- Username: 3-32 chars, alphanumeric + underscore/hyphen, case-insensitive unique
- Email: valid format if provided
- DisplayName: 1-64 chars
- PasswordHash: bcrypt hash, never exposed via API
- IsActive: false disables login but preserves audit history

### ProjectMembership

Existing `project.Member` type is sufficient with minor enhancement:

```go
package project

type Member struct {
    UserID    string    `json:"userId"`
    Role      Role      `json:"role"`
    AddedBy   string    `json:"addedBy"`      // ActorID who added this member
    AddedAt   time.Time `json:"addedAt"`
    UpdatedAt time.Time `json:"updatedAt"`    // Last role change
}
```

**Membership Rules**:
1. A User can be a member of multiple Projects.
2. A User has exactly one Role per Project.
3. Project.Members list is append-only for audit; removal sets a tombstone flag (future enhancement).
4. A Project must have at least one Owner at all times.

### Authentication Credential

Stored separately from User for security isolation.

```go
package auth

import "time"

type Credential struct {
    UserID       string    `json:"userId"`
    PasswordHash string    `json:"passwordHash"` // bcrypt
    UpdatedAt    time.Time `json:"updatedAt"`
}
```

### Session Token (JWT or opaque)

```go
package auth

import "time"

type Token struct {
    TokenID   string    `json:"tokenId"`
    UserID    string    `json:"userId"`
    IssuedAt  time.Time `json:"issuedAt"`
    ExpiresAt time.Time `json:"expiresAt"`
    Revoked   bool      `json:"revoked"`
}
```

**Token Lifecycle**:
- Default TTL: 24 hours
- Refresh: explicit refresh endpoint (optional Phase 2)
- Revocation: explicit logout or admin action

---

## Permission Checking

### Service Layer Pattern

Every service method accepting a user action must check permissions first.

```go
func (s *SessionService) Start(ctx context.Context, actorID, sessionID string) error {
    // 1. Load session
    sess, err := s.sessionRepo.Get(ctx, sessionID)
    if err != nil {
        return err
    }

    // 2. Load actor's project membership
    membership, err := s.permissionSvc.GetMembership(ctx, actorID, sess.ProjectID)
    if err != nil {
        return fmt.Errorf("actor %q is not a member of project %q", actorID, sess.ProjectID)
    }

    // 3. Check permission
    if err := s.permissionSvc.CanStartSession(membership.Role, sess.Type, sess.CreatedBy, actorID); err != nil {
        // 4. Audit denial
        s.auditRepo.Append(ctx, audit.Event{
            ActorID:    actorID,
            Action:     "session.start.denied",
            TargetType: "session",
            TargetID:   sessionID,
            Metadata:   map[string]string{"reason": err.Error()},
        })
        return fmt.Errorf("permission denied: %w", err)
    }

    // 5. Proceed with operation
    // ...
}
```

### PermissionService Interface

```go
package permission

type Service interface {
    // Membership
    GetMembership(ctx context.Context, userID, projectID string) (Membership, error)
    HasRole(ctx context.Context, userID, projectID string, minRole project.Role) bool

    // Project permissions
    CanViewProject(role project.Role) bool
    CanModifyProject(role project.Role) bool
    CanDeleteProject(role project.Role) bool
    CanManageMembers(role project.Role) bool

    // Session permissions
    CanCreateSession(role project.Role, sessionType session.Type) error
    CanStartSession(role project.Role, sessionType session.Type, sessionOwner, actorID string) error
    CanStopSession(role project.Role, sessionType session.Type, sessionOwner, actorID string) error
    CanDeleteSession(role project.Role, sessionType session.Type, sessionOwner, actorID string) error
    CanSendCommand(role project.Role, sessionType session.Type, sessionOwner, actorID string) error

    // Checkpoint permissions
    CanCreateCheckpoint(role project.Role) bool
    CanRestoreCheckpoint(role project.Role, targetSessionType session.Type, targetOwner, actorID string) error
    CanDeleteCheckpoint(role project.Role) bool

    // Artifact permissions
    CanUploadArtifact(role project.Role) bool
    CanApproveArtifact(role project.Role) bool
    CanApplyArtifact(role project.Role, targetSessionType session.Type, targetOwner, actorID string) error
    CanDeleteArtifact(role project.Role) bool

    // Room permissions
    CanModifyRoom(role project.Role) bool
    CanDeleteRoom(role project.Role) bool
}

type Membership struct {
    UserID    string
    ProjectID string
    Role      project.Role
}
```

---

## Implementation Plan

### Phase 1: Domain Models (Week 1)

**Tasks**:
1. Create `internal/user/` package
   - `user.go` — User struct
   - `repository.go` — UserRepository interface
   - `validation.go` — Username/email validation
2. Create `internal/auth/` package
   - `credential.go` — Credential struct
   - `token.go` — Token struct
   - `repository.go` — AuthRepository interface
3. Update `internal/project/project.go`
   - Add `AddedBy`, `AddedAt`, `UpdatedAt` to Member struct
4. Create JSON schemas
   - `schemas/user.json`
   - `schemas/credential.json`
   - `schemas/token.json`

**Deliverables**:
- Domain models pass `go vet`
- Unit tests for validation logic
- JSON schema validation

### Phase 2: Storage Layer (Week 1-2)

**Tasks**:
1. Implement `internal/repository/memory/user_repo.go`
   - In-memory UserRepository (for testing)
2. Implement `internal/storage/filesystem/user_store.go`
   - Filesystem-backed UserRepository
   - File layout: `users/{userID}.json`
3. Implement `internal/storage/filesystem/credential_store.go`
   - File layout: `credentials/{userID}.json` (restricted permissions 0600)
4. Implement `internal/storage/filesystem/token_store.go`
   - File layout: `tokens/{tokenID}.json`

**Deliverables**:
- Repository tests pass
- Atomic write safety verified
- Password hash never written to audit log

### Phase 3: Permission Service (Week 2)

**Tasks**:
1. Expand `internal/permission/service/service.go`
   - Implement all methods from PermissionService interface
   - Add role hierarchy checks
   - Add "own session" ownership logic
2. Write comprehensive permission tests
   - `TestRoleHierarchy`
   - `TestViewerCannotWrite`
   - `TestResearcherCanCreateFork`
   - `TestMaintainerCanStartShared`
   - `TestOwnerCanDeleteProject`

**Deliverables**:
- Permission matrix fully implemented
- 100+ test cases covering all combinations
- Role hierarchy verified

### Phase 4: Authentication Service (Week 2)

**Tasks**:
1. Create `internal/auth/service/service.go`
   - `Register(username, password, email) → User`
   - `Login(username, password) → Token`
   - `Logout(tokenID) → void`
   - `ValidateToken(tokenID) → UserID`
   - `ChangePassword(userID, oldPass, newPass)`
2. Use `golang.org/x/crypto/bcrypt` for password hashing
3. Token generation: use `crypto/rand` for TokenID

**Deliverables**:
- Auth service tests pass
- Bcrypt cost factor: 10 (balance security vs performance)
- Token TTL: 24 hours

### Phase 5: Service Layer Integration (Week 3)

**Tasks**:
1. Add `actorID string` parameter to all service methods
   - `sessionsvc.Start(ctx, actorID, sessionID)`
   - `checkpointsvc.Create(ctx, actorID, req)`
   - `artifactsvc.Approve(ctx, actorID, artifactID)`
2. Inject PermissionService into all services
3. Add permission checks before every state-mutating operation
4. Add permission denial audit events

**Files to modify**:
- `internal/session/service/service.go`
- `internal/checkpoint/service/service.go`
- `internal/artifact/service/service.go`
- `internal/project/service/service.go` (new file)
- `internal/room/service/service.go` (new file)

**Deliverables**:
- All services check permissions
- Integration tests verify permission enforcement
- Audit log contains denial events

### Phase 6: CLI Integration (Week 3)

**Tasks**:
1. Add global flags to `cmd/stratum/main.go`
   - `--user <userID>` or `STRATUM_USER` env var
   - `--token <token>` or `STRATUM_TOKEN` env var
2. Add auth commands
   - `stratum users register --username alice --email alice@example.com`
   - `stratum users login --username alice` (prompt for password)
   - `stratum users logout`
   - `stratum users list` (for project owners)
3. Add membership commands
   - `stratum projects members add --project 117lab --user bob --role researcher`
   - `stratum projects members remove --project 117lab --user bob`
   - `stratum projects members list --project 117lab`
4. Pass actorID to all service calls

**Deliverables**:
- CLI enforces authentication
- Error messages explain permission requirements
- `stratum sessions start` fails with clear message if user lacks permission

### Phase 7: HTTP API Integration (Week 4)

**Tasks**:
1. Add authentication middleware to `internal/api/http/server.go`
   - Extract token from `Authorization: Bearer <token>` header
   - Validate token via AuthService
   - Inject UserID into request context
2. Add endpoints
   - `POST /v1/auth/register` — user registration
   - `POST /v1/auth/login` — returns JWT token
   - `POST /v1/auth/logout` — revokes token
   - `GET /v1/users/me` — current user info
   - `GET /v1/projects/{id}/members` — list members
   - `POST /v1/projects/{id}/members` — add member
   - `DELETE /v1/projects/{id}/members/{userID}` — remove member
3. Update all existing endpoints to use actorID from context

**Deliverables**:
- HTTP API requires Bearer token
- Unauthorized requests return 401
- Forbidden requests return 403 with clear reason

### Phase 8: Testing & Documentation (Week 4)

**Tasks**:
1. Write E2E test scenarios
   - Alice (Owner) creates project → adds Bob (Researcher) → Bob creates fork → Charlie (Viewer) fails to start session
2. Update `docs/architecture.md` — add RBAC section
3. Create `docs/rbac.md` (this document)
4. Update `docs/cli-reference.md` — add user/membership commands
5. Update `README.md` — replace "shared-token" with "user authentication"
6. Add migration guide for existing deployments (shared-token → first user bootstrap)

**Deliverables**:
- E2E tests pass
- Documentation reflects new auth model
- Migration path documented

---

## Security Considerations

### Password Storage
- **Never** store plaintext passwords
- Use `bcrypt` with cost factor 10 (adjustable via config)
- Credential files have filesystem permissions 0600
- Password hash field is never serialized in API responses

### Token Security
- Tokens are cryptographically random (32 bytes → hex)
- Tokens have expiration (default 24h)
- Revoked tokens are checked on every request
- Token store is separate from user store

### Audit Trail
- Every permission denial creates an audit event
- Audit events are append-only (no deletion)
- Audit events include: actorID, action, targetType, targetID, reason
- Failed login attempts are logged

### Session Ownership
- Researcher can only modify their own fork/private/review sessions
- "Own session" means `session.CreatedBy == actorID`
- Shared sessions are never "owned" by a single user

### Bootstrap Problem
- First user registration creates a system admin (not tied to any project)
- System admin can create first project and assign Owner role to themselves
- After bootstrap, system admin role is disabled
- Alternative: pre-seed first user via deployment config

---

## Migration Strategy

### Existing Deployments

Current deployments use shared-token authentication. Migration steps:

1. **Backup existing data**
   ```bash
   cp -r ./data ./data.backup
   ```

2. **Run migration script**
   ```bash
   stratum admin migrate-to-rbac \
     --data-dir ./data \
     --bootstrap-user admin \
     --bootstrap-email admin@example.com
   ```
   - Creates first user with Owner role on all existing projects
   - Generates random password (printed once)
   - All existing audit events get `actorID: "system"` for pre-migration operations

3. **Update client scripts**
   - Replace `--token <shared-token>` with `--user admin --token <new-token>`
   - Or set `STRATUM_USER=admin` and `STRATUM_TOKEN=<token>`

4. **Add real users**
   ```bash
   stratum users register --username alice --email alice@example.com
   stratum projects members add --project 117lab --user alice --role researcher
   ```

### Fresh Deployments

Fresh deployments start with no users. First user registration:

```bash
# Start controller
stratum-controller serve --listen :8080 --data-dir ./data

# Register first user (automatically becomes project creator)
stratum users register --username admin --email admin@example.com

# Create first project
stratum projects create --id 117lab --name "1.17 Testing Lab"

# Admin is automatically added as Owner
```

---

## Future Enhancements (Out of Scope for Phase 6b)

### Phase 7 Candidates

1. **API Keys**
   - Long-lived tokens for automation
   - Scoped permissions (read-only, session-only, etc.)

2. **OAuth2 / OIDC**
   - External identity providers (GitHub, Google, etc.)
   - SSO for enterprise deployments

3. **Group-based permissions**
   - Project groups (e.g., "tmc-researchers", "redstone-team")
   - Assign roles to groups instead of individual users

4. **Fine-grained permissions**
   - Per-room permissions (Alice can manage room A but not room B)
   - Per-session permissions (temporary collaborator access)

5. **Audit log retention policies**
   - Automatic archival of old audit events
   - Compliance with data retention regulations

6. **Rate limiting**
   - Per-user API rate limits
   - Prevent brute-force login attempts

7. **Two-factor authentication (2FA)**
   - TOTP-based 2FA for sensitive operations

---

## Open Questions

1. **Token storage**: JWT (stateless) or opaque tokens (stateful)?
   - **Recommendation**: Opaque tokens stored in filesystem for MVP. JWT for future scalability.

2. **Password reset**: Email-based or admin-initiated?
   - **Recommendation**: Admin-initiated for MVP (no email infrastructure yet).

3. **User deactivation vs deletion**: Soft delete or hard delete?
   - **Recommendation**: Soft delete (set `IsActive: false`) to preserve audit trail.

4. **Bootstrap user**: Auto-create or manual?
   - **Recommendation**: Manual registration via CLI for explicit control.

5. **Role customization**: Fixed roles or custom roles per project?
   - **Recommendation**: Fixed roles for MVP. Custom roles in Phase 7.

---

## Summary

This RBAC design provides:
- ✅ User accounts with password authentication
- ✅ Project membership with four roles (Viewer, Researcher, Maintainer, Owner)
- ✅ Explicit permission checks in all service methods
- ✅ Audit trail for permission denials
- ✅ CLI and HTTP API integration
- ✅ Migration path from shared-token to user auth
- ✅ Foundation for future OAuth2/SSO integration

**Estimated implementation time**: 4 weeks (1 engineer)

**Dependencies**: None (fully independent from Lucy)

**Risk**: Low (standard RBAC pattern, no novel cryptography)
