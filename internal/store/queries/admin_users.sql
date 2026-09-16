-- name: GetAdminUserByUsername :one
SELECT
    id,
    username,
    display_name,
    password_hash,
    role,
    status,
    failed_login_attempts,
    mfa_failed_attempts,
    locked_until,
    mfa_enabled,
    mfa_secret_encrypted,
    mfa_pending_secret_encrypted,
    mfa_pending_expires_at,
    mfa_enrolled_at,
    last_login_at,
    created_at,
    updated_at
FROM admin_users
WHERE username = ?;

-- name: GetAdminUserByID :one
SELECT
    id,
    username,
    display_name,
    password_hash,
    role,
    status,
    failed_login_attempts,
    mfa_failed_attempts,
    locked_until,
    mfa_enabled,
    mfa_secret_encrypted,
    mfa_pending_secret_encrypted,
    mfa_pending_expires_at,
    mfa_enrolled_at,
    last_login_at,
    created_at,
    updated_at
FROM admin_users
WHERE id = ?;

-- name: CountAdminUsers :one
SELECT count(*) FROM admin_users;

-- name: CreateAdminUser :exec
INSERT INTO admin_users (
    id,
    username,
    display_name,
    password_hash,
    role,
    status,
    failed_login_attempts,
    mfa_failed_attempts,
    locked_until,
    mfa_enabled,
    mfa_secret_encrypted,
    mfa_pending_secret_encrypted,
    mfa_pending_expires_at,
    mfa_enrolled_at,
    last_login_at,
    created_at,
    updated_at
) VALUES (
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?
);

-- name: CountFilteredAdminUsers :one
SELECT count(*)
FROM admin_users u
WHERE
    (@filter_role = '' OR u.role = @filter_role)
    AND (@filter_status = '' OR u.status = @filter_status)
    AND (
        @search_query = '' OR
        like(@search_query, u.username, '\') OR
        like(@search_query, u.display_name, '\')
    );

-- name: ListAdminUsers :many
SELECT
    u.id,
    u.username,
    u.display_name,
    u.role,
    u.status,
    u.failed_login_attempts,
    u.mfa_failed_attempts,
    u.locked_until,
    u.mfa_enabled,
    u.mfa_enrolled_at,
    u.last_login_at,
    u.created_at,
    u.updated_at
FROM admin_users u
WHERE
    (@filter_role = '' OR u.role = @filter_role)
    AND (@filter_status = '' OR u.status = @filter_status)
    AND (
        @search_query = '' OR
        like(@search_query, u.username, '\') OR
        like(@search_query, u.display_name, '\')
    )
ORDER BY u.created_at DESC, u.id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateAdminUserRole :execrows
UPDATE admin_users
SET
    role = @new_role,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = @id AND updated_at = @expected_updated_at;

-- name: UpdateAdminUserStatus :execrows
UPDATE admin_users
SET
    status = @new_status,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = @id AND updated_at = @expected_updated_at;

-- name: UpdateAdminUserPassword :exec
UPDATE admin_users
SET
    password_hash = ?,
    failed_login_attempts = 0,
    mfa_failed_attempts = 0,
    locked_until = NULL,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = ?;

-- name: SetAdminUserPendingMFA :exec
UPDATE admin_users
SET
    mfa_pending_secret_encrypted = ?,
    mfa_pending_expires_at = ?,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = ?;

-- name: ConfirmAdminUserMFA :exec
UPDATE admin_users
SET
    mfa_enabled = 1,
    mfa_secret_encrypted = mfa_pending_secret_encrypted,
    mfa_pending_secret_encrypted = '',
    mfa_pending_expires_at = NULL,
    mfa_failed_attempts = 0,
    mfa_enrolled_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now'),
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = ?;

-- name: DisableAdminUserMFA :exec
UPDATE admin_users
SET
    mfa_enabled = 0,
    mfa_secret_encrypted = '',
    mfa_pending_secret_encrypted = '',
    mfa_pending_expires_at = NULL,
    mfa_failed_attempts = 0,
    mfa_enrolled_at = NULL,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = ?;

-- name: ClearAdminUserPendingMFA :exec
UPDATE admin_users
SET
    mfa_pending_secret_encrypted = '',
    mfa_pending_expires_at = NULL,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = ?;

-- name: UpdateAdminUserPasswordSuccess :exec
UPDATE admin_users
SET
    failed_login_attempts = 0,
    last_login_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now'),
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = ?;

-- name: UpdateAdminUserMFASuccess :exec
UPDATE admin_users
SET
    failed_login_attempts = 0,
    mfa_failed_attempts = 0,
    locked_until = NULL,
    last_login_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now'),
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = ?;

-- name: UpdateAdminUserLoginSuccess :exec
UPDATE admin_users
SET
    failed_login_attempts = 0,
    mfa_failed_attempts = 0,
    locked_until = NULL,
    last_login_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now'),
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = ?;

-- name: IncrementAdminUserFailedAttempts :exec
UPDATE admin_users
SET
    failed_login_attempts = failed_login_attempts + 1,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = ?;

-- name: RecordPasswordFailedAttemptAndLock :one
UPDATE admin_users
SET
    failed_login_attempts = failed_login_attempts + 1,
    status = CASE
        WHEN status = 'disabled' THEN 'disabled'
        WHEN (failed_login_attempts + 1) >= @max_attempts THEN 'locked'
        ELSE status
    END,
    locked_until = CASE
        WHEN status = 'disabled' THEN locked_until
        WHEN (failed_login_attempts + 1) >= @max_attempts THEN @locked_until
        ELSE locked_until
    END,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = @id
RETURNING id, username, failed_login_attempts, status, locked_until;

-- name: RecordMFAFailedAttemptAndLock :one
UPDATE admin_users
SET
    mfa_failed_attempts = mfa_failed_attempts + 1,
    status = CASE
        WHEN status = 'disabled' THEN 'disabled'
        WHEN (mfa_failed_attempts + 1) >= @max_attempts THEN 'locked'
        ELSE status
    END,
    locked_until = CASE
        WHEN status = 'disabled' THEN locked_until
        WHEN (mfa_failed_attempts + 1) >= @max_attempts THEN @locked_until
        ELSE locked_until
    END,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = @id
RETURNING id, username, mfa_failed_attempts, status, locked_until;

-- name: LockAdminUser :exec
UPDATE admin_users
SET
    status = 'locked',
    locked_until = ?,
    failed_login_attempts = failed_login_attempts + 1,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = ?;

-- name: UnlockAdminUser :exec
UPDATE admin_users
SET
    status = 'active',
    locked_until = NULL,
    failed_login_attempts = 0,
    mfa_failed_attempts = 0,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = ?;

-- name: CreateAdminSession :exec
INSERT INTO admin_sessions (
    id,
    user_id,
    mfa_verified,
    ip_address,
    user_agent,
    expires_at,
    last_activity_at,
    created_at
) VALUES (
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?
);

-- name: GetAdminSessionWithUser :one
SELECT
    s.id AS session_id,
    s.user_id,
    s.mfa_verified,
    s.ip_address,
    s.user_agent,
    s.expires_at,
    s.last_activity_at,
    s.created_at AS session_created_at,
    u.username,
    u.display_name,
    u.role,
    u.status,
    u.mfa_enabled
FROM admin_sessions s
JOIN admin_users u ON s.user_id = u.id
WHERE s.id = ?;

-- name: UpdateAdminSessionActivity :exec
UPDATE admin_sessions
SET last_activity_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = ?;

-- name: ElevateAdminSessionMFA :exec
UPDATE admin_sessions
SET
    mfa_verified = 1,
    last_activity_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = ?;

-- name: DeleteAdminSession :exec
DELETE FROM admin_sessions
WHERE id = ?;

-- name: DeleteUserAdminSessions :exec
DELETE FROM admin_sessions
WHERE user_id = ?;

-- name: DeleteExpiredAdminSessions :exec
DELETE FROM admin_sessions
WHERE expires_at <= strftime('%Y-%m-%dT%H:%M:%fZ', 'now');

-- name: InsertAdminAuditLog :exec
INSERT INTO admin_audit_logs (
    id,
    user_id,
    username,
    action,
    actor_id,
    actor_username,
    target_id,
    details,
    ip_address,
    created_at
) VALUES (
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?
);

-- name: CountFilteredAdminAuditLogs :one
SELECT count(*)
FROM admin_audit_logs l
WHERE
    (@filter_action = '' OR l.action = @filter_action)
    AND (@filter_actor = '' OR l.actor_username = @filter_actor)
    AND (@filter_period_since = '' OR l.created_at >= @filter_period_since)
    AND (
        @search_query = '' OR
        like(@search_query, l.username, '\') OR
        like(@search_query, l.actor_username, '\') OR
        like(@search_query, l.target_id, '\') OR
        like(@search_query, l.details, '\')
    );

-- name: ListAdminAuditLogs :many
SELECT
    l.id,
    l.user_id,
    l.username,
    l.action,
    l.actor_id,
    l.actor_username,
    l.target_id,
    l.details,
    l.ip_address,
    l.created_at
FROM admin_audit_logs l
WHERE
    (@filter_action = '' OR l.action = @filter_action)
    AND (@filter_actor = '' OR l.actor_username = @filter_actor)
    AND (@filter_period_since = '' OR l.created_at >= @filter_period_since)
    AND (
        @search_query = '' OR
        like(@search_query, l.username, '\') OR
        like(@search_query, l.actor_username, '\') OR
        like(@search_query, l.target_id, '\') OR
        like(@search_query, l.details, '\')
    )
ORDER BY l.created_at DESC, l.id DESC
LIMIT @page_limit OFFSET @page_offset;
