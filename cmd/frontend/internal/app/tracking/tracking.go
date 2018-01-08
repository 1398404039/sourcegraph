package tracking

import (
	"encoding/json"
	"log"
	"net/url"
	"time"

	log15 "gopkg.in/inconshreveable/log15.v2"

	"github.com/pkg/errors"

	"sourcegraph.com/sourcegraph/sourcegraph/cmd/frontend/internal/app/envvar"
	"sourcegraph.com/sourcegraph/sourcegraph/cmd/frontend/internal/app/tracking/slackinternal"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/env"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/hubspot"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/hubspot/hubspotutil"
)

// TrackUser handles user data logging during auth flows
//
// Specifically, updating user data properties in HubSpot
func TrackUser(avatarURL, externalID *string, email, event string) {
	var uid string
	if externalID != nil {
		uid = *externalID
	}

	defer func() {
		if err := recover(); err != nil {
			log.Printf("panic in tracking.TrackUser: %s", err)
		}
	}()

	// If the user is in a dev or on-prem environment, don't do any tracking
	if env.Version == "dev" || !envvar.SourcegraphDotComMode() {
		return
	}

	// Generate a single set of user parameters for HubSpot and Slack exports
	contactParams := &hubspot.ContactProperties{
		UserID:     email,
		UID:        uid,
		LookerLink: lookerUserLink(email),
	}

	// Update or create user contact information in HubSpot
	hsResponse, err := trackHubSpotContact(email, event, contactParams)
	if err != nil {
		log15.Warn("trackHubSpotContact: failed to create or update HubSpot contact on auth", "source", "HubSpot", "error", err)
	}

	// Finally, post signup notification to Slack
	if event == "SignupCompleted" {
		err = slackinternal.NotifyOnSignup(avatarURL, email, contactParams, hsResponse)
		if err != nil {
			log15.Error("Error sending new signup details to Slack", "error", err)
			return
		}
	}
}

func trackHubSpotContact(email string, eventLabel string, params *hubspot.ContactProperties) (*hubspot.ContactResponse, error) {
	if email == "" {
		return nil, errors.New("User must have a valid email address.")
	}

	c, err := hubspotutil.Client()
	if err != nil {
		log15.Warn(err.Error())
		return nil, nil
	}

	if eventLabel == "SignupCompleted" {
		today := time.Now().Truncate(24 * time.Hour)
		// Convert to milliseconds
		params.RegisteredAt = today.UTC().Unix() * 1000
	}

	// Create or update the contact
	resp, err := c.CreateOrUpdateContact(email, params)
	if err != nil {
		return nil, err
	}

	// Log the event if relevant (in this case, for "SignupCompleted" events)
	if _, ok := hubspotutil.EventNameToHubSpotID[eventLabel]; ok {
		err := c.LogEvent(email, hubspotutil.EventNameToHubSpotID[eventLabel], map[string]string{})
		if err != nil {
			return nil, errors.Wrap(err, "LogEvent")
		}
	}

	// Parse response
	hsResponse := &hubspot.ContactResponse{}
	err = json.Unmarshal(resp, hsResponse)
	if err != nil {
		return nil, err
	}

	return hsResponse, nil
}

func lookerUserLink(email string) string {
	return "https://sourcegraph.looker.com/dashboards/9?Email=" + url.QueryEscape(email)
}
