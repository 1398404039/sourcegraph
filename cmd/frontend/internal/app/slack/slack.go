// Package slack is used to send notifications of an organization's activity to a
// given Slack webhook. In contrast with package slackinternal, this package contains
// notifications that external users and customers should also be able to receive.
package slack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	log15 "gopkg.in/inconshreveable/log15.v2"

	"sourcegraph.com/sourcegraph/sourcegraph/cmd/frontend/internal/pkg/types"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/env"

	"github.com/pkg/errors"
)

var sourcegraphOrgWebhookURL = env.Get("SLACK_COMMENTS_BOT_HOOK", "", "Webhook for dogfooding notifications from an organization-level Slack bot.")

// Client is capable of posting a message to a Slack webhook
type Client struct {
	webhookURL            *string
	alsoSendToSourcegraph bool
}

// New creates a new Slack client
func New(webhookURL *string, alsoSendToSourcegraph bool) *Client {
	return &Client{webhookURL: webhookURL, alsoSendToSourcegraph: alsoSendToSourcegraph}
}

// User is an interface for accessing a Sourcegraph user's profile data
type User interface {
	Username() string
	DisplayName() *string
	AvatarURL() *string
}

// Payload is the wrapper for a Slack message, defined at:
// https://api.slack.com/docs/message-formatting
type Payload struct {
	Attachments []*Attachment `json:"attachments,omitempty"`
}

// Attachment is a Slack message attachment, defined at:
// https://api.slack.com/docs/message-attachments
type Attachment struct {
	AuthorIcon string   `json:"author_icon,omitempty"`
	AuthorLink string   `json:"author_link,omitempty"`
	AuthorName string   `json:"author_name,omitempty"`
	Color      string   `json:"color"`
	Fallback   string   `json:"fallback"`
	Fields     []*Field `json:"fields"`
	Footer     string   `json:"footer"`
	MarkdownIn []string `json:"mrkdwn_in"`
	ThumbURL   string   `json:"thumb_url"`
	Text       string   `json:"text,omitempty"`
	Timestamp  int64    `json:"ts"`
	Title      string   `json:"title"`
	TitleLink  string   `json:"title_link,omitempty"`
}

// Field is a single item within an attachment, defined at:
// https://api.slack.com/docs/message-attachments
type Field struct {
	Short bool   `json:"short"`
	Title string `json:"title"`
	Value string `json:"value"`
}

// Post sends payload to a Slack channel defined by the provided webhookURL
// This function should not be called directly — rather, it should be called
// through a helper Notify* function on a slack.Client object.
func Post(payload *Payload, webhookURL *string) error {
	if webhookURL == nil || *webhookURL == "" {
		return nil
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return errors.Wrap(err, "slack: marshal json")
	}
	req, err := http.NewRequest("POST", *webhookURL, bytes.NewReader(payloadJSON))
	if err != nil {
		return errors.Wrap(err, "slack: create post request")
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "slack: http request")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return fmt.Errorf("slack: %s failed with %d %s", payloadJSON, resp.StatusCode, string(body))
	}
	return nil
}

// NotifyOnComment posts a message to the defined Slack channel
// when a user posts a reply to a thread
func (c *Client) NotifyOnComment(
	user User,
	userEmail string,
	org *types.Org,
	orgRepo *types.OrgRepo,
	thread *types.Thread,
	comment *types.Comment,
	recipients []string,
	deepURL string,
	threadTitle string,
) {
	// First, send the uncensored comment to the Client's webhookURL
	err := c.notifyOnComments(false, user, userEmail, org, orgRepo, thread, comment, recipients, deepURL, threadTitle, c.webhookURL, false)
	if err != nil {
		log15.Error("slack.NotifyOnComment failed", "error", err)
	}

	// Next, if the comment was made by an external Sourcegraph customer, ALSO send the
	// comment to the Sourcegraph-internal webhook. In these instances, set censored
	// to true to ensure that the private contents of the comment remain private.
	if c.alsoSendToSourcegraph && sourcegraphOrgWebhookURL != "" && org.Name != "Sourcegraph" {
		err := c.notifyOnComments(false, user, userEmail, org, orgRepo, thread, comment, recipients, deepURL, threadTitle, &sourcegraphOrgWebhookURL, true)
		if err != nil {
			log15.Error("slack.NotifyOnThread failed", "error", err)
		}
	}
}

// NotifyOnThread posts a message to the defined Slack channel
// when a user creates a thread
func (c *Client) NotifyOnThread(
	user User,
	userEmail string,
	org *types.Org,
	orgRepo *types.OrgRepo,
	thread *types.Thread,
	comment *types.Comment,
	recipients []string,
	deepURL string,
) {
	// First, send the uncensored comment to the Client's webhookURL
	err := c.notifyOnComments(true, user, userEmail, org, orgRepo, thread, comment, recipients, deepURL, "", c.webhookURL, false)
	if err != nil {
		log15.Error("slack.NotifyOnThread failed", "error", err)
	}

	// Next, if the comment was made by an external Sourcegraph customer, ALSO send the
	// comment to the Sourcegraph-internal webhook. In these instances, set censored
	// to true to ensure that the private contents of the comment remain private.
	if c.alsoSendToSourcegraph && sourcegraphOrgWebhookURL != "" && org.Name != "Sourcegraph" {
		err := c.notifyOnComments(true, user, userEmail, org, orgRepo, thread, comment, recipients, deepURL, "", &sourcegraphOrgWebhookURL, true)
		if err != nil {
			log15.Error("slack.NotifyOnThread failed", "error", err)
		}
	}
}

func (c *Client) notifyOnComments(
	isNewThread bool,
	user User,
	userEmail string,
	org *types.Org,
	orgRepo *types.OrgRepo,
	thread *types.Thread,
	comment *types.Comment,
	recipients []string,
	deepURL string,
	threadTitle string,
	webhookURL *string,
	censored bool,
) error {
	color := "good"
	actionText := "created a thread"
	if !isNewThread {
		color = "warning"
		if !censored {
			if len(threadTitle) > 75 {
				threadTitle = threadTitle[0:75] + "..."
			}
			actionText = fmt.Sprintf("replied to a thread: \"%s\"", threadTitle)
		} else {
			actionText = fmt.Sprintf("replied to a thread")
		}
	}
	text := "_private_"
	if !censored {
		text = comment.Contents
	}

	displayNameText := userEmail
	if user.DisplayName() != nil {
		displayNameText = *user.DisplayName()
	}
	usernameText := ""
	if user.Username() != "" {
		usernameText = fmt.Sprintf("(@%s) ", user.Username())
	}
	payload := &Payload{
		Attachments: []*Attachment{
			&Attachment{
				AuthorName: fmt.Sprintf("%s %s%s", displayNameText, usernameText, actionText),
				AuthorLink: deepURL,
				Fallback:   fmt.Sprintf("%s %s<%s|%s>!", displayNameText, usernameText, deepURL, actionText),
				Color:      color,
				Fields: []*Field{
					&Field{
						Title: "Org",
						Value: fmt.Sprintf("`%s`\n(%d member(s) notified)", org.Name, len(recipients)),
						Short: true,
					},
				},
				Text:       text,
				MarkdownIn: []string{"text", "fields"},
			},
		},
	}

	if !censored {
		payload.Attachments[0].Fields = append([]*Field{
			&Field{
				Title: "Path",
				Value: fmt.Sprintf("<%s|%s/%s (lines %d–%d)>",
					deepURL,
					orgRepo.CanonicalRemoteID,
					thread.RepoRevisionPath,
					thread.StartLine,
					thread.EndLine),
				Short: true,
			},
		}, payload.Attachments[0].Fields...)
	} else {
		payload.Attachments[0].Fields = append(payload.Attachments[0].Fields,
			&Field{
				Title: "IDs",
				Value: fmt.Sprintf("Comment ID: %d\nThread ID: %d", comment.ID, thread.ID),
				Short: true,
			})
	}

	if user.AvatarURL() != nil {
		payload.Attachments[0].ThumbURL = *user.AvatarURL()
		payload.Attachments[0].AuthorIcon = *user.AvatarURL()
	}

	return Post(payload, webhookURL)
}

// NotifyOnInvite posts a message to the defined Slack channel
// when a user invites another user to join their org
func (c *Client) NotifyOnInvite(user User, userEmail string, org *types.Org, inviteEmail string) {
	displayNameText := userEmail
	if user.DisplayName() != nil {
		displayNameText = *user.DisplayName()
	}
	usernameText := ""
	if user.Username() != "" {
		usernameText = fmt.Sprintf("(@%s) ", user.Username())
	}

	text := fmt.Sprintf("*%s* %sjust invited %s to join *<https://sourcegraph.com/organizations/%s/settings|%s>*", displayNameText, usernameText, inviteEmail, org.Name, org.Name)

	payload := &Payload{
		Attachments: []*Attachment{
			&Attachment{
				Fallback:   text,
				Color:      "#F96316",
				Text:       text,
				MarkdownIn: []string{"text"},
			},
		},
	}

	if user.AvatarURL() != nil {
		payload.Attachments[0].ThumbURL = *user.AvatarURL()
	}

	// First, send the notification to the Client's webhookURL
	err := Post(payload, c.webhookURL)
	if err != nil {
		log15.Error("slack.NotifyOnInvite failed", "error", err)
	}

	// Next, if the action was by an external Sourcegraph customer, also send the
	// notification to the Sourcegraph-internal webhook
	if c.alsoSendToSourcegraph && sourcegraphOrgWebhookURL != "" && org.Name != "Sourcegraph" {
		err := Post(payload, &sourcegraphOrgWebhookURL)
		if err != nil {
			log15.Error("slack.NotifyOnInvite failed", "error", err)
		}
	}
}

// NotifyOnAcceptedInvite posts a message to the defined Slack channel
// when an invited user accepts their invite to join an org
func (c *Client) NotifyOnAcceptedInvite(user User, userEmail string, org *types.Org) {
	displayNameText := userEmail
	if user.DisplayName() != nil {
		displayNameText = *user.DisplayName()
	}
	usernameText := ""
	if user.Username() != "" {
		usernameText = fmt.Sprintf("(@%s) ", user.Username())
	}

	text := fmt.Sprintf("*%s* %sjust accepted their invitation to join *<https://sourcegraph.com/organizations/%s/settings|%s>*", displayNameText, usernameText, org.Name, org.Name)

	payload := &Payload{
		Attachments: []*Attachment{
			&Attachment{
				Fallback:   text,
				Color:      "#B114F7",
				Text:       text,
				MarkdownIn: []string{"text"},
			},
		},
	}

	if user.AvatarURL() != nil {
		payload.Attachments[0].ThumbURL = *user.AvatarURL()
	}

	// First, send the notification to the Client's webhookURL
	err := Post(payload, c.webhookURL)
	if err != nil {
		log15.Error("slack.NotifyOnAcceptedInvite failed", "error", err)
	}

	// Next, if the action was by an external Sourcegraph customer, also send the
	// notification to the Sourcegraph-internal webhook
	if c.alsoSendToSourcegraph && sourcegraphOrgWebhookURL != "" && org.Name != "Sourcegraph" {
		err := Post(payload, &sourcegraphOrgWebhookURL)
		if err != nil {
			log15.Error("slack.NotifyOnAcceptedInvite failed", "error", err)
		}
	}
}
