package productsubscription

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	graphql "github.com/graph-gophers/graphql-go"
	"github.com/graph-gophers/graphql-go/relay"
	"github.com/sourcegraph/enterprise/cmd/frontend/internal/dotcom/billing"
	"github.com/sourcegraph/sourcegraph/cmd/frontend/backend"
	db_ "github.com/sourcegraph/sourcegraph/cmd/frontend/db"
	"github.com/sourcegraph/sourcegraph/cmd/frontend/graphqlbackend"
	"github.com/sourcegraph/sourcegraph/cmd/frontend/graphqlbackend/graphqlutil"
	stripe "github.com/stripe/stripe-go"
	"github.com/stripe/stripe-go/customer"
	"github.com/stripe/stripe-go/event"
	"github.com/stripe/stripe-go/sub"
)

func init() {
	graphqlbackend.ProductSubscriptionByID = func(ctx context.Context, id graphql.ID) (graphqlbackend.ProductSubscription, error) {
		return productSubscriptionByID(ctx, id)
	}
}

// productSubscription implements the GraphQL type ProductSubscription.
type productSubscription struct {
	v *dbSubscription

	once          sync.Once
	billingSub    *stripe.Subscription
	billingSubErr error
}

// productSubscriptionByID looks up and returns the ProductSubscription with the given GraphQL
// ID. If no such ProductSubscription exists, it returns a non-nil error.
func productSubscriptionByID(ctx context.Context, id graphql.ID) (*productSubscription, error) {
	idString, err := unmarshalProductSubscriptionID(id)
	if err != nil {
		return nil, err
	}
	return productSubscriptionByDBID(ctx, idString)
}

// productSubscriptionByDBID looks up and returns the ProductSubscription with the given database
// ID. If no such ProductSubscription exists, it returns a non-nil error.
func productSubscriptionByDBID(ctx context.Context, id string) (*productSubscription, error) {
	v, err := dbSubscriptions{}.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	// 🚨 SECURITY: Only site admins and the subscription account's user may view a product subscription.
	if err := backend.CheckSiteAdminOrSameUser(ctx, v.UserID); err != nil {
		return nil, err
	}
	return &productSubscription{v: v}, nil
}

func (r *productSubscription) ID() graphql.ID {
	return marshalProductSubscriptionID(r.v.ID)
}

func marshalProductSubscriptionID(id string) graphql.ID {
	return relay.MarshalID("ProductSubscription", id)
}

func unmarshalProductSubscriptionID(id graphql.ID) (productSubscriptionID string, err error) {
	err = relay.UnmarshalSpec(id, &productSubscriptionID)
	return
}

func (r *productSubscription) Name() string {
	return fmt.Sprintf("L-%s", strings.ToUpper(strings.Replace(r.v.ID, "-", "", -1)[:10]))
}

func (r *productSubscription) Account(ctx context.Context) (*graphqlbackend.UserResolver, error) {
	return graphqlbackend.UserByIDInt32(ctx, r.v.UserID)
}

// getBillingSubscription returns the subscription from the billing system. If there is no
// associated subscription on the billing system, it returns (nil, nil).
func (r *productSubscription) getBillingSubscription(ctx context.Context) (*stripe.Subscription, error) {
	if r.v.BillingSubscriptionID == nil {
		return nil, nil
	}
	r.once.Do(func() {
		r.billingSub, r.billingSubErr = sub.Get(*r.v.BillingSubscriptionID, &stripe.SubscriptionParams{Params: stripe.Params{Context: ctx}})
	})
	return r.billingSub, r.billingSubErr
}

func (r *productSubscription) Plan(ctx context.Context) (graphqlbackend.ProductPlan, error) {
	billingSub, err := r.getBillingSubscription(ctx)
	if billingSub == nil || err != nil {
		return nil, err
	}
	return billing.ToProductPlan(billingSub.Plan)
}

func (r *productSubscription) UserCount(ctx context.Context) (*int32, error) {
	billingSub, err := r.getBillingSubscription(ctx)
	if billingSub == nil || err != nil {
		return nil, err
	}
	userCount := int32(billingSub.Quantity)
	return &userCount, nil
}

func (r *productSubscription) ExpiresAt(ctx context.Context) (*string, error) {
	billingSub, err := r.getBillingSubscription(ctx)
	if billingSub == nil || err != nil {
		return nil, err
	}
	s := time.Unix(billingSub.CurrentPeriodEnd, 0).Format(time.RFC3339)
	return &s, nil
}

func (r *productSubscription) Events(ctx context.Context) ([]graphqlbackend.ProductSubscriptionEvent, error) {
	if r.v.BillingSubscriptionID == nil {
		return []graphqlbackend.ProductSubscriptionEvent{}, nil
	}

	// List all events related to this subscription. The related_object parameter is an undocumented
	// Stripe API.
	params := &stripe.EventListParams{
		ListParams: stripe.ListParams{Context: ctx},
	}
	params.Filters.AddFilter("related_object", "", *r.v.BillingSubscriptionID)
	events := event.List(params)
	var gqlEvents []graphqlbackend.ProductSubscriptionEvent
	for events.Next() {
		gqlEvent, okToShowUser := billing.ToProductSubscriptionEvent(events.Event())
		if okToShowUser {
			gqlEvents = append(gqlEvents, gqlEvent)
		}
	}
	if err := events.Err(); err != nil {
		return nil, err
	}
	return gqlEvents, nil
}

func (r *productSubscription) ActiveLicense(ctx context.Context) (graphqlbackend.ProductLicense, error) {
	// Return newest license.
	licenses, err := dbLicenses{}.List(ctx, dbLicensesListOptions{
		ProductSubscriptionID: r.v.ID,
		LimitOffset:           &db_.LimitOffset{Limit: 1},
	})
	if err != nil {
		return nil, err
	}
	if len(licenses) == 0 {
		return nil, nil
	}
	return &productLicense{v: licenses[0]}, nil
}

func (r *productSubscription) ProductLicenses(ctx context.Context, args *graphqlutil.ConnectionArgs) (graphqlbackend.ProductLicenseConnection, error) {
	// 🚨 SECURITY: Only site admins may list historical product licenses (to reduce confusion
	// around old license reuse). Other viewers should use ProductSubscription.activeLicense.
	if err := backend.CheckCurrentUserIsSiteAdmin(ctx); err != nil {
		return nil, err
	}

	opt := dbLicensesListOptions{ProductSubscriptionID: r.v.ID}
	args.Set(&opt.LimitOffset)
	return &productLicenseConnection{opt: opt}, nil
}

func (r *productSubscription) CreatedAt() string {
	return r.v.CreatedAt.Format(time.RFC3339)
}

func (r *productSubscription) IsArchived() bool { return r.v.ArchivedAt != nil }

func (r *productSubscription) URL(ctx context.Context) (string, error) {
	accountUser, err := r.Account(ctx)
	if err != nil {
		return "", err
	}
	return accountUser.URL() + "/subscriptions/" + string(r.ID()), nil
}

func (r *productSubscription) URLForSiteAdmin(ctx context.Context) *string {
	// 🚨 SECURITY: Only site admins may see this URL. Currently it does not contain any sensitive
	// info, but there is no need to show it to non-site admins.
	if err := backend.CheckCurrentUserIsSiteAdmin(ctx); err != nil {
		return nil
	}
	u := fmt.Sprintf("/site-admin/dotcom/product/subscriptions/%s", r.ID())
	return &u
}

func (r *productSubscription) URLForSiteAdminBilling(ctx context.Context) (*string, error) {
	// 🚨 SECURITY: Only site admins may see this URL, which might contain the subscription's billing ID.
	if err := backend.CheckCurrentUserIsSiteAdmin(ctx); err != nil {
		return nil, err
	}
	if id := r.v.BillingSubscriptionID; id != nil {
		u := billing.SubscriptionURL(*id)
		return &u, nil
	}
	return nil, nil
}

func (ProductSubscriptionLicensingResolver) CreateProductSubscription(ctx context.Context, args *graphqlbackend.CreateProductSubscriptionArgs) (graphqlbackend.ProductSubscription, error) {
	// 🚨 SECURITY: Only site admins may create product subscriptions.
	if err := backend.CheckCurrentUserIsSiteAdmin(ctx); err != nil {
		return nil, err
	}

	user, err := graphqlbackend.UserByID(ctx, args.AccountID)
	if err != nil {
		return nil, err
	}
	id, err := dbSubscriptions{}.Create(ctx, user.SourcegraphID())
	if err != nil {
		return nil, err
	}
	return productSubscriptionByDBID(ctx, id)
}

func (ProductSubscriptionLicensingResolver) SetProductSubscriptionBilling(ctx context.Context, args *graphqlbackend.SetProductSubscriptionBillingArgs) (*graphqlbackend.EmptyResponse, error) {
	// 🚨 SECURITY: Only site admins may update product subscriptions.
	if err := backend.CheckCurrentUserIsSiteAdmin(ctx); err != nil {
		return nil, err
	}

	// Ensure the args refer to valid subscriptions in the database and in the billing system.
	dbSub, err := productSubscriptionByID(ctx, args.ID)
	if err != nil {
		return nil, err
	}
	if args.BillingSubscriptionID != nil {
		if _, err := sub.Get(*args.BillingSubscriptionID, &stripe.SubscriptionParams{Params: stripe.Params{Context: ctx}}); err != nil {
			return nil, err
		}
	}

	stringValue := func(s *string) string {
		if s == nil {
			return ""
		}
		return *s
	}

	if err := (dbSubscriptions{}).Update(ctx, dbSub.v.ID, dbSubscriptionUpdate{
		billingSubscriptionID: &sql.NullString{
			String: stringValue(args.BillingSubscriptionID),
			Valid:  args.BillingSubscriptionID != nil,
		},
	}); err != nil {
		return nil, err
	}
	return &graphqlbackend.EmptyResponse{}, nil
}

func (ProductSubscriptionLicensingResolver) CreatePaidProductSubscription(ctx context.Context, args *graphqlbackend.CreatePaidProductSubscriptionArgs) (*graphqlbackend.CreatePaidProductSubscriptionResult, error) {
	user, err := graphqlbackend.UserByID(ctx, args.AccountID)
	if err != nil {
		return nil, err
	}

	// 🚨 SECURITY: Users may only create paid product subscriptions for themselves. Site admins may
	// create them for any user.
	if err := backend.CheckSiteAdminOrSameUser(ctx, user.SourcegraphID()); err != nil {
		return nil, err
	}

	// Determine which license tags to use for the purchased plan. Do this early on because it's the
	// most likely place for a stupid mistake to cause a bug, and doing it early means the user
	// hasn't been charged if there is an error.
	licenseTags, err := billing.LicenseTagsForProductPlan(ctx, args.ProductSubscription.Plan)
	if err != nil {
		return nil, err
	}

	// Create the subscription in our database first, before processing payment. If payment fails,
	// users can retry payment on the already created subscription.
	subID, err := dbSubscriptions{}.Create(ctx, user.SourcegraphID())
	if err != nil {
		return nil, err
	}

	// Get the billing customer for the current user, and update it to use the payment source
	// provided to us.
	custID, err := billing.GetOrAssignUserCustomerID(ctx, user.SourcegraphID())
	if err != nil {
		return nil, err
	}
	custUpdateParams := &stripe.CustomerParams{
		Params: stripe.Params{Context: ctx},
	}
	custUpdateParams.SetSource(args.PaymentToken)
	cust, err := customer.Update(custID, custUpdateParams)
	if err != nil {
		return nil, err
	}

	// Create the billing subscription.
	billingSub, err := sub.New(&stripe.SubscriptionParams{
		Params:   stripe.Params{Context: ctx},
		Customer: stripe.String(cust.ID),
		Items: []*stripe.SubscriptionItemsParams{
			{
				Plan:     stripe.String(args.ProductSubscription.Plan),
				Quantity: stripe.Int64(int64(args.ProductSubscription.UserCount)),
			},
		},
	})
	if err != nil {
		return nil, err
	}

	// Link the billing subscription with the subscription in our database.
	if err := (dbSubscriptions{}).Update(ctx, subID, dbSubscriptionUpdate{
		billingSubscriptionID: &sql.NullString{
			String: billingSub.ID,
			Valid:  true,
		},
	}); err != nil {
		return nil, err
	}

	// Generate a new license key for the subscription.
	if _, err := generateProductLicenseForSubscription(ctx, subID, &graphqlbackend.ProductLicenseInput{
		Tags:      licenseTags,
		UserCount: args.ProductSubscription.UserCount,
		ExpiresAt: int32(billingSub.CurrentPeriodEnd),
	}); err != nil {
		return nil, err
	}

	sub, err := productSubscriptionByDBID(ctx, subID)
	if err != nil {
		return nil, err
	}
	return &graphqlbackend.CreatePaidProductSubscriptionResult{ProductSubscriptionValue: sub}, nil
}

func (ProductSubscriptionLicensingResolver) ArchiveProductSubscription(ctx context.Context, args *graphqlbackend.ArchiveProductSubscriptionArgs) (*graphqlbackend.EmptyResponse, error) {
	// 🚨 SECURITY: Only site admins may update product subscriptions.
	if err := backend.CheckCurrentUserIsSiteAdmin(ctx); err != nil {
		return nil, err
	}

	sub, err := productSubscriptionByID(ctx, args.ID)
	if err != nil {
		return nil, err
	}
	if err := (dbSubscriptions{}).Archive(ctx, sub.v.ID); err != nil {
		return nil, err
	}
	return &graphqlbackend.EmptyResponse{}, nil
}

func (ProductSubscriptionLicensingResolver) ProductSubscriptions(ctx context.Context, args *graphqlbackend.ProductSubscriptionsArgs) (graphqlbackend.ProductSubscriptionConnection, error) {
	var accountUser *graphqlbackend.UserResolver
	if args.Account != nil {
		var err error
		accountUser, err = graphqlbackend.UserByID(ctx, *args.Account)
		if err != nil {
			return nil, err
		}
	}

	// 🚨 SECURITY: Users may only list their own product subscriptions. Site admins may list
	// licenses for all users, or for any other user.
	if accountUser == nil {
		if err := backend.CheckCurrentUserIsSiteAdmin(ctx); err != nil {
			return nil, err
		}
	} else {
		if err := backend.CheckSiteAdminOrSameUser(ctx, accountUser.SourcegraphID()); err != nil {
			return nil, err
		}
	}

	var opt dbSubscriptionsListOptions
	if accountUser != nil {
		opt.UserID = accountUser.SourcegraphID()
	}
	args.ConnectionArgs.Set(&opt.LimitOffset)
	return &productSubscriptionConnection{opt: opt}, nil
}

// productSubscriptionConnection implements the GraphQL type ProductSubscriptionConnection.
//
// 🚨 SECURITY: When instantiating a productSubscriptionConnection value, the caller MUST
// check permissions.
type productSubscriptionConnection struct {
	opt dbSubscriptionsListOptions

	// cache results because they are used by multiple fields
	once    sync.Once
	results []*dbSubscription
	err     error
}

func (r *productSubscriptionConnection) compute(ctx context.Context) ([]*dbSubscription, error) {
	r.once.Do(func() {
		opt2 := r.opt
		if opt2.LimitOffset != nil {
			tmp := *opt2.LimitOffset
			opt2.LimitOffset = &tmp
			opt2.Limit++ // so we can detect if there is a next page
		}

		r.results, r.err = dbSubscriptions{}.List(ctx, opt2)
	})
	return r.results, r.err
}

func (r *productSubscriptionConnection) Nodes(ctx context.Context) ([]graphqlbackend.ProductSubscription, error) {
	results, err := r.compute(ctx)
	if err != nil {
		return nil, err
	}

	var l []graphqlbackend.ProductSubscription
	for _, result := range results {
		l = append(l, &productSubscription{v: result})
	}
	return l, nil
}

func (r *productSubscriptionConnection) TotalCount(ctx context.Context) (int32, error) {
	count, err := dbSubscriptions{}.Count(ctx, r.opt)
	return int32(count), err
}

func (r *productSubscriptionConnection) PageInfo(ctx context.Context) (*graphqlutil.PageInfo, error) {
	results, err := r.compute(ctx)
	if err != nil {
		return nil, err
	}
	return graphqlutil.HasNextPage(r.opt.LimitOffset != nil && len(results) > r.opt.Limit), nil
}
