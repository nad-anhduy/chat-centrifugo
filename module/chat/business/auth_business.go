package business

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"be-chat-centrifugo/module/chat/model"
	"be-chat-centrifugo/pkg/deviceprint"
	"be-chat-centrifugo/pkg/hashing"
	"be-chat-centrifugo/pkg/jwt"
	"be-chat-centrifugo/pkg/userkeypair"
)

type AuthBusiness struct {
	userStore UserStorage
	jwtSecret string
	masterKey string
}

func NewAuthBusiness(userStore UserStorage, jwtSecret, masterKey string) *AuthBusiness {
	return &AuthBusiness{
		userStore: userStore,
		jwtSecret: jwtSecret,
		masterKey: masterKey,
	}
}

// RegisterReq registers a user; RSA keys are generated on the server.
type RegisterReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// RegisterResp is returned to the client after successful registration (private key is only delivered on login).
type RegisterResp struct {
	PublicKey string `json:"public_key"`
}

func (biz *AuthBusiness) Register(ctx context.Context, req *RegisterReq) (*RegisterResp, error) {
	if biz.masterKey == "" {
		return nil, errors.New("MASTER_KEY is not configured")
	}

	pubPEM, privPEM, err := userkeypair.GenerateRSA2048PEM()
	if err != nil {
		return nil, fmt.Errorf("generate rsa keypair: %w", err)
	}

	encPriv, err := userkeypair.EncryptPEMWithMasterKey(biz.masterKey, privPEM)
	if err != nil {
		return nil, fmt.Errorf("encrypt private key: %w", err)
	}

	hashed, err := hashing.HashPassword(req.Password)
	if err != nil {
		return nil, err
	}

	user := &model.User{
		Username:     req.Username,
		PasswordHash: hashed,
		PublicKey:    pubPEM,
		PrivateKey:   encPriv,
	}

	if err := biz.userStore.CreateUser(ctx, user); err != nil {
		return nil, err
	}

	return &RegisterResp{PublicKey: pubPEM}, nil
}

type LoginReq struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	UserAgent string `json:"-"`
	ClientIP  string `json:"-"`
}

// LoginResp includes JWT and decrypted RSA private key PEM for the Web Crypto client.
type LoginResp struct {
	Token      string `json:"token"`
	PrivateKey string `json:"private_key"`
	PublicKey  string `json:"public_key"`
}

func (biz *AuthBusiness) Login(ctx context.Context, req *LoginReq) (*LoginResp, error) {
	if biz.masterKey == "" {
		return nil, errors.New("MASTER_KEY is not configured")
	}

	user, err := biz.userStore.GetUserByUsername(ctx, req.Username)
	if err != nil {
		return nil, errors.New("user not found")
	}

	if !hashing.CheckPasswordHash(req.Password, user.PasswordHash) {
		return nil, errors.New("invalid password")
	}

	if user.PrivateKey == "" {
		return nil, errors.New("account has no server-side private key; re-register or restore from backup")
	}

	privPEM, err := userkeypair.DecryptPEMWithMasterKey(biz.masterKey, user.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("unlock private key: %w", err)
	}

	token, err := jwt.GenerateToken(user.ID, biz.jwtSecret)
	if err != nil {
		return nil, err
	}

	fp := deviceprint.Fingerprint(req.UserAgent, req.ClientIP)
	if err := biz.recordDeviceOnLogin(ctx, user.ID, fp, req.UserAgent, req.ClientIP); err != nil {
		return nil, err
	}

	return &LoginResp{
		Token:      token,
		PrivateKey: privPEM,
		PublicKey:  user.PublicKey,
	}, nil
}

func (biz *AuthBusiness) recordDeviceOnLogin(ctx context.Context, userID, fingerprint, userAgent, ip string) error {
	ok, err := biz.userStore.HasActiveUserDevice(ctx, userID, fingerprint)
	if err != nil {
		return err
	}
	if ok {
		return biz.userStore.UpdateUserDeviceLastLogin(ctx, userID, fingerprint)
	}

	dev := &model.UserDevice{
		UserID:            userID,
		DeviceFingerprint: fingerprint,
		UserAgent:         userAgent,
		IP:                ip,
		LastLogin:         time.Now(),
		Status:            model.UserDeviceStatusActive,
	}
	if err := biz.userStore.InsertUserDevice(ctx, dev); err != nil {
		return err
	}

	newInfo, _ := json.Marshal(map[string]string{
		"device_fingerprint": fingerprint,
		"user_agent":         userAgent,
		"ip":                 ip,
	})
	ch := &model.UserDeviceChanged{
		UserID:  userID,
		OldInfo: "{}",
		NewInfo: string(newInfo),
	}
	return biz.userStore.InsertUserDeviceChanged(ctx, ch)
}
