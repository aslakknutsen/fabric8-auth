package token

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	config "github.com/fabric8-services/fabric8-auth/configuration"
	"github.com/fabric8-services/fabric8-auth/resource"
	testsuite "github.com/fabric8-services/fabric8-auth/test/suite"

	"github.com/dgrijalva/jwt-go"
	"github.com/goadesign/goa"
	"github.com/satori/go.uuid"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type TokenWhiteBoxTest struct {
	testsuite.RemoteTestSuite
	manager *tokenManager
}

func TestRunTokenWhiteBoxTest(t *testing.T) {
	suite.Run(t, &TokenWhiteBoxTest{RemoteTestSuite: testsuite.NewRemoteTestSuite()})
}

func (s *TokenWhiteBoxTest) SetupSuite() {
	resource.Require(s.T(), resource.Remote)
	var err error
	s.Config, err = config.GetConfigurationData()
	if err != nil {
		panic(fmt.Errorf("Failed to setup the configuration: %s", err.Error()))
	}
	m, err := NewManager(s.Config)
	require.Nil(s.T(), err)
	require.NotNil(s.T(), m)
	tm, ok := m.(*tokenManager)
	require.True(s.T(), ok)
	s.manager = tm
}

func (s *TokenWhiteBoxTest) TestKeycloakTokensLoaded() {
	minKeyNumber := 2 // At least one service account key and one Keycloak key
	_, serviceAccountKid := s.Config.GetServiceAccountPrivateKey()
	require.NotEqual(s.T(), "", serviceAccountKid)
	require.NotNil(s.T(), s.manager.PublicKey(serviceAccountKid))

	_, dServiceAccountKid := s.Config.GetDeprecatedServiceAccountPrivateKey()
	if dServiceAccountKid != "" {
		minKeyNumber++
		require.NotNil(s.T(), s.manager.PublicKey(dServiceAccountKid))
	}
	require.True(s.T(), len(s.manager.PublicKeys()) >= minKeyNumber)

	require.Equal(s.T(), len(s.manager.publicKeys), len(s.manager.PublicKeys()))
	require.Equal(s.T(), len(s.manager.publicKeys), len(s.manager.publicKeysMap))
	for i, k := range s.manager.publicKeys {
		require.NotEqual(s.T(), "", k.KeyID)
		require.NotNil(s.T(), s.manager.PublicKey(k.KeyID))
		require.Equal(s.T(), s.manager.PublicKeys()[i], k.Key)
	}

	jwKeys := s.manager.JsonWebKeys()
	require.NotEmpty(s.T(), jwKeys.Keys)

	pemKeys := s.manager.PemKeys()
	require.NotEmpty(s.T(), pemKeys.Keys)
}

func (s *TokenWhiteBoxTest) TestServiceAccount() {
	r := &goa.RequestData{
		Request: &http.Request{Host: "example.com"},
	}

	tokenString, err := s.manager.ServiceAccountToken(r)
	require.Nil(s.T(), err)

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		kid := token.Header["kid"]
		if kid == nil {
			return nil, errors.New("There is no 'kid' header in the token")
		}
		if fmt.Sprintf("%s", kid) != s.manager.serviceAccountPrivateKey.KeyID {
			return nil, errors.New(fmt.Sprintf("The key ID %s doesn't match the private key ID %s", kid, s.manager.serviceAccountPrivateKey.KeyID))
		}
		key := s.manager.PublicKey(fmt.Sprintf("%s", kid))
		if key == nil {
			return nil, errors.New(fmt.Sprintf("There is no public key with such ID: %s", kid))
		}
		return key, nil
	})
	require.Nil(s.T(), err)

	claims := token.Claims.(jwt.MapClaims)
	require.Equal(s.T(), ServiceAccountID, claims["sub"])
	require.Equal(s.T(), "auth", claims["service_accountname"])
	jti, ok := claims["jti"].(string)
	require.True(s.T(), ok)
	_, err = uuid.FromString(jti)
	require.Nil(s.T(), err)
	require.NotEmpty(s.T(), claims["iat"])
	require.Equal(s.T(), "http://example.com", claims["iss"])
}
