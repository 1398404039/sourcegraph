package invite

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/sourcegraph/sourcegraph/cmd/frontend/internal/pkg/types"
	"github.com/sourcegraph/sourcegraph/pkg/conf"
	"github.com/sourcegraph/sourcegraph/pkg/txemail"
)

type TokenPayload struct {
	OrgID   int32
	OrgName string
	Email   string
}

func getSecretKey() ([]byte, error) {
	encoded := conf.Get().SecretKey
	if encoded == "" {
		return nil, errors.New("secret key is not set in site config")
	}
	v, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.New("error base64-decoding secret key")
	}
	return v, err
}

func ParseToken(tokenString string) (*TokenPayload, error) {
	payload, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return 0, fmt.Errorf("invite: unexpected signing method %v", token.Header["alg"])
		}
		return getSecretKey()
	})
	if err != nil {
		return nil, err
	}
	claims, ok := payload.Claims.(jwt.MapClaims)
	if !ok || !payload.Valid {
		return nil, errors.New("invite: invalid token")
	}

	orgID, ok := claims["orgID"].(float64)
	if !ok {
		return nil, errors.New("invite: unexpected org id")
	}
	orgName, ok := claims["orgName"].(string)
	if !ok {
		return nil, errors.New("invite: unexpected org name")
	}
	email, ok := claims["email"].(string)
	if !ok {
		return nil, errors.New("invite: unexpected email")
	}

	return &TokenPayload{OrgID: int32(orgID), OrgName: orgName, Email: email}, nil
}

func CreateOrgToken(email string, org *types.Org) (string, error) {
	payload := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"email":   email,
		"orgID":   org.ID,
		"orgName": org.Name, // So the accept invite UI can display the name of the org
		"exp":     time.Now().Add(time.Hour * 24 * 7).Unix(),
	})
	key, err := getSecretKey()
	if err != nil {
		return "", err
	}
	return payload.SignedString(key)
}

func SendEmail(inviteEmail, fromName, orgName, inviteURL string) error {
	return txemail.Send(context.Background(), txemail.Message{
		To:       []string{inviteEmail},
		Template: emailTemplates,
		Data: struct {
			FromName string
			OrgName  string
			URL      string
		}{
			FromName: fromName,
			OrgName:  orgName,
			URL:      inviteURL,
		},
	})
}

var (
	emailTemplates = txemail.MustValidate(txemail.Templates{
		Subject: `{{.FromName}} invited you to join {{.OrgName}} on Sourcegraph`,
		Text: `
{{.FromName}} invited you to join {{.OrgName}} on Sourcegraph.

To accept the invitation, follow this link:

  {{.URL}}
`,
		HTML: `
<p>
  <strong>{{.FromName}}</strong> invited you to join
  <strong>{{.OrgName}}</strong> on Sourcegraph.
</p>

<p><strong><a href="{{.URL}}">Accept the invitation</a></strong></p>
`,
	})
)
