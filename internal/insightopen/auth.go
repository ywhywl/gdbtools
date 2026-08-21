package insightopen

import (
	"encoding/base64"
	"flag"
	"fmt"
	"strings"
)

type Auth struct {
	Username    string
	PasswordB64 string
}

type AuthFlags struct {
	Username    string
	Password    string
	PasswordB64 string
}

func AddAuthFlags(fs *flag.FlagSet, flags *AuthFlags) {
	fs.StringVar(&flags.Username, "insight-user", "", "Insight 登录用户名")
	fs.StringVar(&flags.Password, "insight-password", "", "Insight 登录密码明文")
	fs.StringVar(&flags.PasswordB64, "insight-password-b64", "", "Insight 登录密码 base64")
}

func ResolveAuth(flags AuthFlags) (Auth, error) {
	username := strings.TrimSpace(flags.Username)
	if username == "" {
		return Auth{}, fmt.Errorf("必须提供 --insight-user")
	}

	if strings.TrimSpace(flags.PasswordB64) != "" {
		return Auth{
			Username:    username,
			PasswordB64: strings.TrimSpace(flags.PasswordB64),
		}, nil
	}
	if flags.Password == "" {
		return Auth{}, fmt.Errorf("必须提供 --insight-password 或 --insight-password-b64")
	}
	return Auth{
		Username:    username,
		PasswordB64: base64.StdEncoding.EncodeToString([]byte(flags.Password)),
	}, nil
}
