package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/runner/requesttemplate"
)

func runTemplate(args []string) error {
	if len(args) == 0 {
		return errors.New("missing template command")
	}
	switch args[0] {
	case "render":
		return runTemplateRender(args[1:])
	default:
		return fmt.Errorf("unknown template command: %s", args[0])
	}
}

func runTemplateRender(args []string) error {
	flags := flag.NewFlagSet("template render", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	profilePath := flags.String("profile", "", "Profile bundle path or installed profile id")
	profileHome := flags.String("profile-home", "", "Installed profile bundle home")
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	templateID := flags.String("template", "", "Request template id")
	fixtureID := flags.String("fixture", "", "Fixture id")
	if err := flags.Parse(args); err != nil {
		return err
	}
	bundle, cleanup, err := loadTemplateRenderBundle(context.Background(), *profilePath, *profileHome, *storeRef, *storeURL, *templateID)
	if err != nil {
		return err
	}
	defer cleanup()
	rendered, err := requesttemplate.Render(bundle, requesttemplate.Options{
		TemplateID: *templateID,
		FixtureID:  *fixtureID,
	})
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(rendered)
}

func loadTemplateRenderBundle(ctx context.Context, profileRef string, profileHomeRef string, storeRef string, legacyStoreURL string, templateID string) (profile.Bundle, func(), error) {
	resolvedStoreURL, err := resolveRequiredDailyStoreReference(storeRef, legacyStoreURL)
	if err != nil {
		return profile.Bundle{}, func() {}, err
	}
	if strings.TrimSpace(profileRef) != "" {
		resolvedProfile, err := resolveProfileReference(profileRef, profileHomeRef)
		if err != nil {
			return profile.Bundle{}, func() {}, err
		}
		bundle, err := profile.Load(resolvedProfile)
		return bundle, func() {}, err
	}
	runtime, err := openStore(ctx, resolvedStoreURL)
	if err != nil {
		return profile.Bundle{}, func() {}, err
	}
	bundle, err := serveBundle(ctx, runtime)
	if err != nil {
		closeCLIStore(runtime)
		return profile.Bundle{}, func() {}, err
	}
	if templateNeedsPublishedProfile(bundle, templateID) {
		if catalogIndex, err := runtime.GetProfileCatalogIndex(ctx); err == nil && strings.TrimSpace(catalogIndex.ProfileID) != "" {
			if profileIndex, err := runtime.GetProfileIndex(ctx, catalogIndex.ProfileID); err == nil && strings.TrimSpace(profileIndex.BundlePath) != "" {
				if pathBundle, err := profile.Load(profileIndex.BundlePath); err == nil {
					bundle = pathBundle
				}
			}
		}
	}
	return bundle, cleanupCLIStore(runtime), nil
}

func templateNeedsPublishedProfile(bundle profile.Bundle, templateID string) bool {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return false
	}
	for _, item := range bundle.RequestTemplates {
		if item.ID != templateID {
			continue
		}
		return strings.TrimSpace(item.Method) == "" || strings.TrimSpace(item.Path) == ""
	}
	return false
}
