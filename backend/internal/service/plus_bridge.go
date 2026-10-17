// plus_bridge.go — [plus] exposes internal service helpers for the backend/internal/plus package.
// This file lives in package service so it can access unexported fields.
// Keep it minimal: only add what plus modules actually need.
package service

import (
	"context"
	"database/sql"
	"errors"

	entsql "entgo.io/ent/dialect/sql"
)

// GetGroupByID exposes groupRepo.GetByID for the plus package.
func GetGroupByID(ctx context.Context, svc *APIKeyService, groupID int64) (*Group, error) {
	return svc.groupRepo.GetByID(ctx, groupID)
}

// GetUserByID exposes userRepo.GetByID for the plus package.
func GetUserByID(ctx context.Context, svc *APIKeyService, userID int64) (*User, error) {
	return svc.userRepo.GetByID(ctx, userID)
}

// CanUserBindGroup exposes the unexported canUserBindGroup for the plus package.
func CanUserBindGroup(ctx context.Context, svc *APIKeyService, user *User, group *Group) bool {
	return svc.canUserBindGroup(ctx, user, group)
}

// CheckBalanceEligibility mirrors the gateway's balance-mode rule (balance > 0, plus the
// configured minimum reserve) but reads the user record directly, so the plus package does
// not need BillingCacheService wired in.
func CheckBalanceEligibility(user *User, minimumReserve float64) error {
	if user == nil {
		return ErrInsufficientBalance
	}
	if user.Balance <= 0 {
		return ErrInsufficientBalance
	}
	if minimumReserve > 0 && user.Balance < minimumReserve {
		return ErrInsufficientBalance
	}
	return nil
}

// CheckSubscriptionEligibility mirrors the gateway's subscription-mode rule: an active,
// unexpired subscription that is within the group's daily/weekly/monthly limits.
func CheckSubscriptionEligibility(ctx context.Context, sub *SubscriptionService, userID int64, group *Group) error {
	if sub == nil || group == nil {
		return ErrSubscriptionInvalid
	}
	subscription, err := sub.GetActiveSubscription(ctx, userID, group.ID)
	if err != nil || subscription == nil {
		return ErrSubscriptionInvalid
	}
	_, err = sub.ValidateAndCheckLimits(subscription, group)
	return err
}

// GetRawSetting exposes settingRepo.GetValue so plus modules can read their own storage.
func GetRawSetting(ctx context.Context, svc *SettingService, key string) (string, error) {
	return svc.settingRepo.GetValue(ctx, key)
}

// SetRawSetting exposes settingRepo.Set so plus modules can persist their own storage.
func SetRawSetting(ctx context.Context, svc *SettingService, key, value string) error {
	return svc.settingRepo.Set(ctx, key, value)
}

// FindKeyByName finds a user's API key in a given group by exact name match.
// Returns nil, nil when no matching key exists.
func FindKeyByName(ctx context.Context, svc *APIKeyService, userID, groupID int64, name string) (*APIKey, error) {
	keys, err := svc.apiKeyRepo.SearchAPIKeys(ctx, userID, name, 20)
	if err != nil {
		return nil, err
	}
	for i := range keys {
		k := &keys[i]
		if k.Name == name && k.GroupID != nil && *k.GroupID == groupID {
			return k, nil
		}
	}
	return nil, nil
}

// CreateDedicatedKey creates an auto-managed key: no quota, no expiry, no IP restrictions.
func CreateDedicatedKey(ctx context.Context, svc *APIKeyService, userID int64, name string, groupID *int64) (*APIKey, error) {
	return svc.Create(ctx, userID, CreateAPIKeyRequest{
		Name:    name,
		GroupID: groupID,
	})
}

// PlusSQLDB exposes the underlying *sql.DB of the ent client so plus modules can own their tables.
func PlusSQLDB(sub *SubscriptionService) (*sql.DB, error) {
	if sub == nil || sub.entClient == nil {
		return nil, errors.New("plus: subscription service has no ent client")
	}
	drv, ok := sub.entClient.Driver().(*entsql.Driver)
	if !ok {
		return nil, errors.New("plus: ent driver does not expose *sql.DB")
	}
	return drv.DB(), nil
}

// PlusBalanceInvalidator invalidates balance-related caches after a plus module changes users.balance
// directly — the same two caches RedeemService clears after a balance redeem.
type PlusBalanceInvalidator struct {
	Sub    *SubscriptionService
	APIKey *APIKeyService
}

func (p PlusBalanceInvalidator) InvalidateUserBalance(ctx context.Context, userID int64) {
	if p.Sub != nil && p.Sub.billingCacheService != nil {
		_ = p.Sub.billingCacheService.InvalidateUserBalance(ctx, userID)
	}
	if p.APIKey != nil {
		p.APIKey.InvalidateAuthCacheByUserID(ctx, userID)
	}
}
