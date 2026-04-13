package business

import (
	"context"
	"errors"

	"be-chat-centrifugo/module/chat/model"
	"be-chat-centrifugo/pkg/hashing"
	"be-chat-centrifugo/pkg/jwt"
)

type AuthBusiness struct {
	userStore UserStorage
	jwtSecret string
}

func NewAuthBusiness(userStore UserStorage, jwtSecret string) *AuthBusiness {
	return &AuthBusiness{
		userStore: userStore,
		jwtSecret: jwtSecret,
	}
}

type RegisterReq struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	PublicKey string `json:"public_key"`
}

func (biz *AuthBusiness) Register(ctx context.Context, req *RegisterReq) error {
	hashed, err := hashing.HashPassword(req.Password)
	if err != nil {
		return err
	}

	user := &model.User{
		Username:     req.Username,
		PasswordHash: hashed,
		PublicKey:    req.PublicKey,
	}

	return biz.userStore.CreateUser(ctx, user)
}

type LoginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (biz *AuthBusiness) Login(ctx context.Context, req *LoginReq) (string, error) {
	user, err := biz.userStore.GetUserByUsername(ctx, req.Username)
	if err != nil {
		return "", errors.New("user not found")
	}

	if !hashing.CheckPasswordHash(req.Password, user.PasswordHash) {
		return "", errors.New("invalid password")
	}

	token, err := jwt.GenerateToken(user.ID, biz.jwtSecret)
	if err != nil {
		return "", err
	}

	return token, nil
}
