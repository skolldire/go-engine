package cognito

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"
	qrcode "github.com/skip2/go-qrcode"
)

func (c *Client) RespondToMFAChallenge(ctx context.Context, req MFAChallengeRequest) (*AuthTokens, error) {
	if err := validateMFAChallengeRequest(req); err != nil {
		return nil, err
	}

	ctx, cancel := c.ensureContextWithTimeout(ctx)
	defer cancel()

	challengeParams := map[string]string{
		"USERNAME":                req.Username,
		"SOFTWARE_TOKEN_MFA_CODE": req.MFACode,
	}

	if req.ChallengeType == MFAChallengeTypeSMS {
		challengeParams = map[string]string{
			"USERNAME":     req.Username,
			"SMS_MFA_CODE": req.MFACode,
		}
	}

	input := &cognitoidentityprovider.RespondToAuthChallengeInput{
		ClientId:           aws.String(c.config.ClientID),
		ChallengeName:      types.ChallengeNameType(string(req.ChallengeType)),
		Session:            aws.String(req.SessionToken),
		ChallengeResponses: challengeParams,
	}

	var result *cognitoidentityprovider.RespondToAuthChallengeOutput
	_, err := c.executeOperation(ctx, "RespondToMFAChallenge", func() (interface{}, error) {
		var err error
		result, err = c.cognitoClient.RespondToAuthChallenge(ctx, input)
		return result, err
	})

	if err != nil {
		return nil, handleCognitoError(err)
	}

	if result.AuthenticationResult == nil {
		return nil, fmt.Errorf("authentication result is nil")
	}

	safeString := func(s *string) string {
		if s == nil {
			return ""
		}
		return *s
	}

	if result.AuthenticationResult.AccessToken == nil {
		return nil, fmt.Errorf("access token is nil")
	}
	if result.AuthenticationResult.IdToken == nil {
		return nil, fmt.Errorf("id token is nil")
	}

	tokens := &AuthTokens{
		AccessToken:  safeString(result.AuthenticationResult.AccessToken),
		RefreshToken: safeString(result.AuthenticationResult.RefreshToken),
		IDToken:      safeString(result.AuthenticationResult.IdToken),
		TokenType:    "Bearer",
		ExpiresIn:    int64(result.AuthenticationResult.ExpiresIn),
	}

	if c.logging {
		c.logger.Info(ctx, "MFA challenge completed successfully",
			map[string]interface{}{
				"challenge_type": req.ChallengeType,
			})
	}

	return tokens, nil
}

func (c *Client) AssociateSoftwareToken(ctx context.Context, accessToken string) (*SoftwareTokenAssociation, error) {
	if accessToken == "" {
		return nil, ErrInvalidAccessToken
	}

	_, err := c.ValidateToken(ctx, accessToken)
	if err != nil {
		return nil, ErrInvalidToken
	}

	ctx, cancel := c.ensureContextWithTimeout(ctx)
	defer cancel()

	input := &cognitoidentityprovider.AssociateSoftwareTokenInput{
		AccessToken: aws.String(accessToken),
	}

	var result *cognitoidentityprovider.AssociateSoftwareTokenOutput
	_, err = c.executeOperation(ctx, "AssociateSoftwareToken", func() (interface{}, error) {
		var err error
		result, err = c.cognitoClient.AssociateSoftwareToken(ctx, input)
		return result, err
	})

	if err != nil {
		var enableErr *types.EnableSoftwareTokenMFAException
		if errors.As(err, &enableErr) {
			return nil, ErrMFAAlreadyEnabled
		}
		return nil, handleCognitoError(err)
	}

	if result.SecretCode == nil {
		return nil, fmt.Errorf("secret code not returned from Cognito")
	}
	if result.Session == nil {
		return nil, fmt.Errorf("session token not returned from Cognito")
	}

	secretCode := *result.SecretCode
	session := *result.Session

	user, err := c.GetUserByAccessToken(ctx, accessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}

	issuer := "Valkyr Platform"
	accountName := user.Email
	if accountName == "" {
		accountName = user.Username
	}

	totpURL := fmt.Sprintf("otpauth://totp/%s:%s?secret=%s&issuer=%s",
		issuer, accountName, secretCode, issuer)

	qrCodePNG, err := qrcode.Encode(totpURL, qrcode.Medium, 256)
	if err != nil {
		return nil, fmt.Errorf("failed to generate QR code: %w", err)
	}

	qrCodeBase64 := base64.StdEncoding.EncodeToString(qrCodePNG)

	if c.logging {
		c.logger.Info(ctx, "Software token associated successfully",
			map[string]interface{}{
				"user_id": user.ID,
			})
	}

	return &SoftwareTokenAssociation{
		SecretCode: secretCode,
		QRCode:     qrCodeBase64,
		Session:    session,
	}, nil
}

func (c *Client) VerifySoftwareToken(ctx context.Context, accessToken, userCode, session string) error {
	if accessToken == "" {
		return ErrInvalidAccessToken
	}
	if userCode == "" {
		return ErrMissingRequiredField
	}
	if session == "" {
		return ErrMissingRequiredField
	}

	_, err := c.ValidateToken(ctx, accessToken)
	if err != nil {
		return ErrInvalidToken
	}

	ctx, cancel := c.ensureContextWithTimeout(ctx)
	defer cancel()

	input := &cognitoidentityprovider.VerifySoftwareTokenInput{
		AccessToken: aws.String(accessToken),
		UserCode:    aws.String(userCode),
		Session:     aws.String(session),
	}

	_, err = c.executeOperation(ctx, "VerifySoftwareToken", func() (interface{}, error) {
		return c.cognitoClient.VerifySoftwareToken(ctx, input)
	})

	if err != nil {
		var codeMismatch *types.CodeMismatchException
		if errors.As(err, &codeMismatch) {
			return ErrMFACodeMismatch
		}

		var invalidCode *types.InvalidParameterException
		if errors.As(err, &invalidCode) {
			if invalidCode.Message != nil && strings.Contains(*invalidCode.Message, "code") {
				return ErrInvalidMFACode
			}
		}

		return handleCognitoError(err)
	}

	if c.logging {
		c.logger.Info(ctx, "Software token verified successfully", nil)
	}

	return nil
}

func (c *Client) SetUserMFAPreference(ctx context.Context, accessToken string, smsEnabled, totpEnabled bool) error {
	if accessToken == "" {
		return ErrInvalidAccessToken
	}

	_, err := c.ValidateToken(ctx, accessToken)
	if err != nil {
		return ErrInvalidToken
	}

	ctx, cancel := c.ensureContextWithTimeout(ctx)
	defer cancel()

	preferredSMS := smsEnabled && !totpEnabled
	preferredTOTP := totpEnabled && !smsEnabled
	if smsEnabled && totpEnabled {
		preferredTOTP = true
		preferredSMS = false
	}

	input := &cognitoidentityprovider.SetUserMFAPreferenceInput{
		AccessToken: aws.String(accessToken),
		SoftwareTokenMfaSettings: &types.SoftwareTokenMfaSettingsType{
			Enabled:      totpEnabled,
			PreferredMfa: preferredTOTP,
		},
		SMSMfaSettings: &types.SMSMfaSettingsType{
			Enabled:      smsEnabled,
			PreferredMfa: preferredSMS,
		},
	}

	_, err = c.executeOperation(ctx, "SetUserMFAPreference", func() (interface{}, error) {
		return c.cognitoClient.SetUserMFAPreference(ctx, input)
	})

	if err != nil {
		var invalidParam *types.InvalidParameterException
		if errors.As(err, &invalidParam) {
			if invalidParam.Message != nil && strings.Contains(*invalidParam.Message, "TOTP") {
				return ErrMFAConfigurationRequired
			}
		}
		return handleCognitoError(err)
	}

	if c.logging {
		c.logger.Info(ctx, "User MFA preference updated",
			map[string]interface{}{
				"sms_enabled":  smsEnabled,
				"totp_enabled": totpEnabled,
			})
	}

	return nil
}

func (c *Client) GetUserMFAStatus(ctx context.Context, accessToken string) (*MFAStatus, error) {
	if accessToken == "" {
		return nil, ErrInvalidAccessToken
	}

	_, err := c.ValidateToken(ctx, accessToken)
	if err != nil {
		return nil, ErrInvalidToken
	}

	ctx, cancel := c.ensureContextWithTimeout(ctx)
	defer cancel()

	input := &cognitoidentityprovider.GetUserInput{
		AccessToken: aws.String(accessToken),
	}

	var result *cognitoidentityprovider.GetUserOutput
	_, err = c.executeOperation(ctx, "GetUserMFAStatus", func() (interface{}, error) {
		var err error
		result, err = c.cognitoClient.GetUser(ctx, input)
		return result, err
	})

	if err != nil {
		return nil, handleCognitoError(err)
	}

	mfaStatus := &MFAStatus{
		MFAEnabled:      false,
		MFATypes:        []string{},
		PreferredMethod: "",
		SMSEnabled:      false,
		TOTPEnabled:     false,
	}

	if len(result.UserMFASettingList) > 0 {
		mfaStatus.MFAEnabled = true
		for _, setting := range result.UserMFASettingList {
			switch setting {
			case "SOFTWARE_TOKEN_MFA":
				mfaStatus.TOTPEnabled = true
				if !contains(mfaStatus.MFATypes, "SOFTWARE_TOKEN_MFA") {
					mfaStatus.MFATypes = append(mfaStatus.MFATypes, "SOFTWARE_TOKEN_MFA")
				}
			case "SMS_MFA":
				mfaStatus.SMSEnabled = true
				if !contains(mfaStatus.MFATypes, "SMS_MFA") {
					mfaStatus.MFATypes = append(mfaStatus.MFATypes, "SMS_MFA")
				}
			}
		}

		if mfaStatus.PreferredMethod == "" && len(mfaStatus.MFATypes) > 0 {
			mfaStatus.PreferredMethod = mfaStatus.MFATypes[0]
		}
	}

	if !mfaStatus.MFAEnabled && len(result.MFAOptions) > 0 {
		mfaStatus.MFAEnabled = true
		for _, option := range result.MFAOptions {
			switch option.DeliveryMedium {
			case types.DeliveryMediumTypeSms:
				mfaStatus.SMSEnabled = true
				if !contains(mfaStatus.MFATypes, "SMS_MFA") {
					mfaStatus.MFATypes = append(mfaStatus.MFATypes, "SMS_MFA")
				}
			}
		}

		if mfaStatus.PreferredMethod == "" && len(mfaStatus.MFATypes) > 0 {
			mfaStatus.PreferredMethod = mfaStatus.MFATypes[0]
		}
	}

	if c.logging {
		c.logger.Info(ctx, "User MFA status retrieved",
			map[string]interface{}{
				"mfa_enabled":      mfaStatus.MFAEnabled,
				"mfa_types":        mfaStatus.MFATypes,
				"preferred_method": mfaStatus.PreferredMethod,
			})
	}

	return mfaStatus, nil
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
