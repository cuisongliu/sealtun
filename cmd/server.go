package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/labring/sealtun/pkg/accesspolicy"
	tunnelprotocol "github.com/labring/sealtun/pkg/protocol"
	"github.com/labring/sealtun/pkg/publicauth"
	"github.com/labring/sealtun/pkg/tunnel"
	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:    "server",
	Short:  "Run the tunnel server component (internal)",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("sealtun server component starting on port %d\n", bindPort)

		resolvedSecret, err := resolveServerSecret(secret, secretEnv, os.Getenv)
		if err != nil {
			return err
		}
		if resolvedSecret == "" {
			return fmt.Errorf("secret is required to run the server component")
		}
		if bindPort < 1 || bindPort > 65535 {
			return fmt.Errorf("invalid --port %d: must be between 1 and 65535", bindPort)
		}
		if err := tunnelprotocol.ValidateServer(serverProtocol); err != nil {
			return err
		}
		serverProtocol = tunnelprotocol.Normalize(serverProtocol)
		if err := validateLocalPort(serverLocalPort); err != nil {
			return fmt.Errorf("invalid --local-port: %w", err)
		}
		basicAuth, err := resolveServerBasicAuth(serverBasicAuthInput{
			User:              serverBasicAuthUser,
			UserEnv:           serverBasicAuthUserEnv,
			PasswordHash:      serverBasicAuthPasswordHash,
			PasswordHashEnv:   serverBasicAuthPasswordHashEnv,
			PasswordSHA256:    serverBasicAuthPasswordSHA256,
			PasswordSHA256Env: serverBasicAuthPasswordSHA256Env,
		}, os.Getenv)
		if err != nil {
			return err
		}
		accessPolicy, err := resolveServerAccessPolicy(serverAccessPolicy, serverAccessPolicyEnv, os.Getenv)
		if err != nil {
			return err
		}

		svr := tunnel.NewServerWithOptions(resolvedSecret, bindPort, serverProtocol, serverLocalPort, tunnel.ServerOptions{
			BasicAuth:    basicAuth,
			AccessPolicy: accessPolicy,
		})
		return svr.Start()
	},
}

var secret string
var secretEnv string
var bindPort int
var serverProtocol string
var serverLocalPort string
var serverBasicAuthUser string
var serverBasicAuthUserEnv string
var serverBasicAuthPasswordHash string
var serverBasicAuthPasswordHashEnv string
var serverBasicAuthPasswordSHA256 string
var serverBasicAuthPasswordSHA256Env string
var serverAccessPolicy string
var serverAccessPolicyEnv string

func init() {
	rootCmd.AddCommand(serverCmd)
	serverCmd.Flags().StringVar(&secret, "secret", "", "Tunnel authentication secret")
	serverCmd.Flags().StringVar(&secretEnv, "secret-env", "", "Read tunnel authentication secret from an environment variable")
	serverCmd.Flags().IntVar(&bindPort, "port", 8080, "Port to bind the server to")
	serverCmd.Flags().StringVar(&serverProtocol, "protocol", "https", "Tunnel protocol")
	serverCmd.Flags().StringVar(&serverLocalPort, "local-port", "", "Expected local port displayed on fallback pages")
	serverCmd.Flags().StringVar(&serverBasicAuthUser, "basic-auth-user", "", "Basic Auth username for public tunnel traffic")
	serverCmd.Flags().StringVar(&serverBasicAuthUserEnv, "basic-auth-user-env", "", "Read Basic Auth username from an environment variable")
	serverCmd.Flags().StringVar(&serverBasicAuthPasswordHash, "basic-auth-password-hash", "", "Basic Auth password hash for public tunnel traffic")
	serverCmd.Flags().StringVar(&serverBasicAuthPasswordHashEnv, "basic-auth-password-hash-env", "", "Read Basic Auth password hash from an environment variable")
	serverCmd.Flags().StringVar(&serverBasicAuthPasswordSHA256, "basic-auth-password-sha256", "", "Deprecated: Basic Auth password SHA-256 hex digest for public tunnel traffic")
	serverCmd.Flags().StringVar(&serverBasicAuthPasswordSHA256Env, "basic-auth-password-sha256-env", "", "Deprecated: read Basic Auth password SHA-256 hex digest from an environment variable")
	serverCmd.Flags().StringVar(&serverAccessPolicy, "access-policy", "", "Access policy JSON for public tunnel traffic")
	serverCmd.Flags().StringVar(&serverAccessPolicyEnv, "access-policy-env", "", "Read access policy JSON from an environment variable")
	_ = serverCmd.Flags().MarkHidden("basic-auth-password-sha256")
	_ = serverCmd.Flags().MarkHidden("basic-auth-password-sha256-env")
}

func resolveServerSecret(flagSecret, envName string, lookupEnv func(string) string) (string, error) {
	if envName == "" {
		return flagSecret, nil
	}
	value := lookupEnv(envName)
	if value == "" {
		return "", fmt.Errorf("secret environment variable %s is empty or unset", envName)
	}
	return value, nil
}

func resolveServerValue(flagValue, envName, label string, lookupEnv func(string) string) (string, error) {
	if envName == "" {
		return flagValue, nil
	}
	value := lookupEnv(envName)
	if value == "" {
		return "", fmt.Errorf("%s environment variable %s is empty or unset", label, envName)
	}
	return value, nil
}

type serverBasicAuthInput struct {
	User              string
	UserEnv           string
	PasswordHash      string
	PasswordHashEnv   string
	PasswordSHA256    string
	PasswordSHA256Env string
}

func resolveServerBasicAuth(input serverBasicAuthInput, lookupEnv func(string) string) (*publicauth.BasicAuth, error) {
	hashProvided := input.PasswordHash != "" || input.PasswordHashEnv != ""
	legacyHashProvided := input.PasswordSHA256 != "" || input.PasswordSHA256Env != ""
	if hashProvided && legacyHashProvided {
		return nil, fmt.Errorf("basic auth password hash and deprecated SHA-256 hash cannot be used together")
	}
	username, err := resolveServerValue(input.User, input.UserEnv, "basic auth username", lookupEnv)
	if err != nil {
		return nil, err
	}
	passwordHash, err := resolveServerValue(input.PasswordHash, input.PasswordHashEnv, "basic auth password hash", lookupEnv)
	if err != nil {
		return nil, err
	}
	if passwordHash == "" {
		passwordHash, err = resolveServerValue(input.PasswordSHA256, input.PasswordSHA256Env, "basic auth password hash", lookupEnv)
		if err != nil {
			return nil, err
		}
	}
	if username == "" && passwordHash == "" {
		return nil, nil
	}
	if username == "" || passwordHash == "" {
		return nil, fmt.Errorf("both basic auth username and password hash are required")
	}
	config := publicauth.BasicAuth{Username: username, PasswordHash: passwordHash}
	if err := publicauth.Validate(config); err != nil {
		return nil, err
	}
	return &config, nil
}

func resolveServerAccessPolicy(flagValue, envName string, lookupEnv func(string) string) (*accesspolicy.Policy, error) {
	value, err := resolveServerValue(flagValue, envName, "access policy", lookupEnv)
	if err != nil {
		return nil, err
	}
	if value == "" {
		return nil, nil
	}
	var policy accesspolicy.Policy
	if err := json.Unmarshal([]byte(value), &policy); err != nil {
		return nil, fmt.Errorf("parse access policy: %w", err)
	}
	if err := accesspolicy.Validate(&policy); err != nil {
		return nil, fmt.Errorf("invalid access policy: %w", err)
	}
	return &policy, nil
}
