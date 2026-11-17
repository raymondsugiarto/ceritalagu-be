// Package user is the reference module. Copy its shape for new modules:
//
//	user.go        domain entity, named after the module (no JSON tags, no framework imports)
//	dto.go         <Action><Entity>Input (handler payloads) + <Entity>Output (response data) + mapping
//	repository.go  storage interface + implementation(s)
//	service.go     business rules; returns *apperror.Error for client-facing failures
//	handler.go     HTTP: bind -> call service -> write response. No business logic.
//	router.go      RegisterRoutes: paths + module middleware
package user

import "time"

// Status is the account state (user.status).
type Status string

// User statuses. A guest who only filled email + phone on the last questionnaire step is a draft
// user: they have songs but no password yet. Admin-created users are active.
const (
	StatusDraft     Status = "draft"
	StatusActive    Status = "active"
	StatusSuspended Status = "suspended"
)

// Type is the customer lifecycle stage (user.customer_type). It only moves forward (Promote).
// "guest" (anonymous visitor) has no user row; it exists only as a guest_session.
type Type string

// Customer types, in funnel order.
const (
	TypeLead           Type = "lead"            // gave email + phone, no password yet
	TypeProspect       Type = "prospect"        // set a password or started a checkout
	TypeCustomer       Type = "customer"        // first paid order
	TypeRepeatCustomer Type = "repeat_customer" // two or more paid orders
)

// Types lists every stored type in funnel order.
var Types = []Type{TypeLead, TypeProspect, TypeCustomer, TypeRepeatCustomer}

// Rank is the funnel position (0 for an unknown or empty type).
func (t Type) Rank() int {
	for i, x := range Types {
		if x == t {
			return i + 1
		}
	}
	return 0
}

// User is the domain entity.
type User struct {
	ID     string
	Name   string
	Email  string
	Phone  string
	Status Status
	Type   Type
	// HasPassword and PaidOrders are read-only facts for the admin list (from user_credential and
	// payment_order); repositories fill them on List only.
	HasPassword bool
	PaidOrders  int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ListParams is the repository-level query. Offset/limit are derived from page/page size by the service.
type ListParams struct {
	Search string
	Type   Type // empty = all
	Offset int
	Limit  int
}
