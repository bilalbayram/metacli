package cmd

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/bilalbayram/metacli/internal/auth"
	"github.com/bilalbayram/metacli/internal/config"
)

var profileAuthPreflight = runProfileAuthPreflight

type ProfileCredentials struct {
	Name      string
	Profile   config.Profile
	Token     string
	AppSecret string
}

func loadProfileCredentials(profile string) (*ProfileCredentials, error) {
	if strings.TrimSpace(profile) == "" {
		return nil, errors.New("profile is required")
	}

	configPath, err := config.DefaultPath()
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	name, selected, err := cfg.ResolveProfile(profile)
	if err != nil {
		return nil, err
	}

	if err := profileAuthPreflight(name, configPath); err != nil {
		return nil, fmt.Errorf("auth preflight failed for profile %q: %w", name, err)
	}

	store := auth.NewKeychainStore()
	token, err := store.Get(selected.TokenRef)
	if err != nil {
		return nil, err
	}
	out := &ProfileCredentials{
		Name:    name,
		Profile: selected,
		Token:   token,
	}

	if selected.AppSecretRef != "" {
		appSecret, err := store.Get(selected.AppSecretRef)
		if err != nil {
			return nil, fmt.Errorf("load app secret for profile %q: %w", profile, err)
		}
		out.AppSecret = appSecret
	}
	return out, nil
}

func runProfileAuthPreflight(profile string, configPath string) error {
	if strings.TrimSpace(profile) == "" {
		return errors.New("profile is required")
	}
	if strings.TrimSpace(configPath) == "" {
		return errors.New("config path is required")
	}

	svc := auth.NewService(configPath, auth.NewKeychainStore(), nil, auth.DefaultGraphBaseURL)
	ctx := context.Background()

	for _, methodName := range []string{"PreflightProfile", "Preflight"} {
		handled, err := invokeProfilePreflightMethod(ctx, svc, methodName, profile)
		if handled {
			return err
		}
	}

	_, err := svc.ValidateProfile(ctx, profile)
	return err
}

func invokeProfilePreflightMethod(ctx context.Context, svc any, methodName string, profile string) (bool, error) {
	method := reflect.ValueOf(svc).MethodByName(methodName)
	if !method.IsValid() {
		return false, nil
	}

	methodType := method.Type()
	if methodType.NumIn() == 0 {
		return true, fmt.Errorf("auth preflight method %s has unsupported signature", methodName)
	}

	args := make([]reflect.Value, 0, methodType.NumIn())
	profileAssigned := false
	for i := 0; i < methodType.NumIn(); i++ {
		argType := methodType.In(i)
		if argType.Implements(contextType) {
			args = append(args, reflect.ValueOf(ctx))
			continue
		}
		switch {
		case argType.Kind() == reflect.String:
			args = append(args, reflect.ValueOf(profile).Convert(argType))
			profileAssigned = true
		case argType.Kind() == reflect.Struct:
			value := reflect.New(argType).Elem()
			if !assignProfileToStruct(value, profile) {
				return true, fmt.Errorf("auth preflight method %s input does not expose profile field", methodName)
			}
			args = append(args, value)
			profileAssigned = true
		case argType.Kind() == reflect.Pointer && argType.Elem().Kind() == reflect.Struct:
			value := reflect.New(argType.Elem())
			if !assignProfileToStruct(value.Elem(), profile) {
				return true, fmt.Errorf("auth preflight method %s input does not expose profile field", methodName)
			}
			args = append(args, value)
			profileAssigned = true
		default:
			return true, fmt.Errorf("auth preflight method %s has unsupported signature", methodName)
		}
	}
	if !profileAssigned {
		return true, fmt.Errorf("auth preflight method %s has unsupported signature", methodName)
	}

	_, err := parseAuthMethodResult(method.Call(args))
	if err != nil {
		return true, err
	}
	return true, nil
}

func assignProfileToStruct(target reflect.Value, profile string) bool {
	for _, fieldName := range []string{"Profile", "ProfileName", "Name"} {
		field := target.FieldByName(fieldName)
		if !field.IsValid() || !field.CanSet() || field.Kind() != reflect.String {
			continue
		}
		field.SetString(profile)
		return true
	}
	return false
}
