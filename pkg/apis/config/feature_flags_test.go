/*
Copyright 2020 The Tekton Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config_test

import (
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tektoncd/pipeline/pkg/apis/config"
	test "github.com/tektoncd/pipeline/pkg/reconciler/testing"
	"github.com/tektoncd/pipeline/test/diff"
)

func TestNewFeatureFlagsFromConfigMap(t *testing.T) {
	type testCase struct {
		expectedConfig *config.FeatureFlags
		fileName       string
	}

	testCases := []testCase{
		{
			expectedConfig: &config.FeatureFlags{
				DisableAffinityAssistant:         false,
				RunningInEnvWithInjectedSidecars: true,
				RequireGitSSHSecretKnownHosts:    false,

				DisableCredsInit:       config.DefaultDisableCredsInit,
				AwaitSidecarReadiness:  config.DefaultAwaitSidecarReadiness,
				EnableTektonOCIBundles: config.DefaultEnableTektonOciBundles,
				EnableCustomTasks:      config.DefaultEnableCustomTasks,
				EnableAPIFields:        config.DefaultEnableAPIFields,
				SendCloudEventsForRuns: config.DefaultSendCloudEventsForRuns,
				EmbeddedStatus:         config.DefaultEmbeddedStatus,
			},
			fileName: config.GetFeatureFlagsConfigName(),
		},
		{
			expectedConfig: &config.FeatureFlags{
				DisableAffinityAssistant:         true,
				RunningInEnvWithInjectedSidecars: false,
				AwaitSidecarReadiness:            false,
				RequireGitSSHSecretKnownHosts:    true,
				EnableTektonOCIBundles:           true,
				EnableCustomTasks:                true,
				EnableAPIFields:                  "alpha",
				SendCloudEventsForRuns:           true,
				EmbeddedStatus:                   "both",
			},
			fileName: "feature-flags-all-flags-set",
		},
		{
			expectedConfig: &config.FeatureFlags{
				EnableAPIFields: "alpha",
				// These are prescribed as true by enabling "alpha" API fields, even
				// if the submitted text value is "false".
				EnableTektonOCIBundles: true,
				EnableCustomTasks:      true,

				DisableAffinityAssistant:         config.DefaultDisableAffinityAssistant,
				DisableCredsInit:                 config.DefaultDisableCredsInit,
				RunningInEnvWithInjectedSidecars: config.DefaultRunningInEnvWithInjectedSidecars,
				AwaitSidecarReadiness:            config.DefaultAwaitSidecarReadiness,
				RequireGitSSHSecretKnownHosts:    config.DefaultRequireGitSSHSecretKnownHosts,
				SendCloudEventsForRuns:           config.DefaultSendCloudEventsForRuns,
				EmbeddedStatus:                   config.DefaultEmbeddedStatus,
			},
			fileName: "feature-flags-enable-api-fields-overrides-bundles-and-custom-tasks",
		},
		{
			expectedConfig: &config.FeatureFlags{
				EnableAPIFields:        "stable",
				EnableTektonOCIBundles: true,
				EnableCustomTasks:      true,

				DisableAffinityAssistant:         config.DefaultDisableAffinityAssistant,
				DisableCredsInit:                 config.DefaultDisableCredsInit,
				RunningInEnvWithInjectedSidecars: config.DefaultRunningInEnvWithInjectedSidecars,
				AwaitSidecarReadiness:            config.DefaultAwaitSidecarReadiness,
				RequireGitSSHSecretKnownHosts:    config.DefaultRequireGitSSHSecretKnownHosts,
				SendCloudEventsForRuns:           config.DefaultSendCloudEventsForRuns,
				EmbeddedStatus:                   config.DefaultEmbeddedStatus,
			},
			fileName: "feature-flags-bundles-and-custom-tasks",
		},
	}

	for _, tc := range testCases {
		fileName := tc.fileName
		expectedConfig := tc.expectedConfig
		t.Run(fileName, func(t *testing.T) {
			verifyConfigFileWithExpectedFeatureFlagsConfig(t, fileName, expectedConfig)
		})
	}
}

func TestNewFeatureFlagsFromEmptyConfigMap(t *testing.T) {
	FeatureFlagsConfigEmptyName := "feature-flags-empty"
	expectedConfig := &config.FeatureFlags{
		DisableAffinityAssistant:         config.DefaultDisableAffinityAssistant,
		DisableCredsInit:                 config.DefaultDisableCredsInit,
		RunningInEnvWithInjectedSidecars: config.DefaultRunningInEnvWithInjectedSidecars,
		AwaitSidecarReadiness:            config.DefaultAwaitSidecarReadiness,
		RequireGitSSHSecretKnownHosts:    config.DefaultRequireGitSSHSecretKnownHosts,
		EnableTektonOCIBundles:           config.DefaultEnableTektonOciBundles,
		EnableCustomTasks:                config.DefaultEnableCustomTasks,
		EnableAPIFields:                  config.DefaultEnableAPIFields,
		SendCloudEventsForRuns:           config.DefaultSendCloudEventsForRuns,
		EmbeddedStatus:                   config.DefaultEmbeddedStatus,
	}
	verifyConfigFileWithExpectedFeatureFlagsConfig(t, FeatureFlagsConfigEmptyName, expectedConfig)
}

func TestGetFeatureFlagsConfigName(t *testing.T) {
	for _, tc := range []struct {
		description         string
		featureFlagEnvValue string
		expected            string
	}{{
		description:         "Feature flags config value not set",
		featureFlagEnvValue: "",
		expected:            "feature-flags",
	}, {
		description:         "Feature flags config value set",
		featureFlagEnvValue: "feature-flags-test",
		expected:            "feature-flags-test",
	}} {
		t.Run(tc.description, func(t *testing.T) {
			original := os.Getenv("CONFIG_FEATURE_FLAGS_NAME")
			defer t.Cleanup(func() {
				os.Setenv("CONFIG_FEATURE_FLAGS_NAME", original)
			})
			if tc.featureFlagEnvValue != "" {
				os.Setenv("CONFIG_FEATURE_FLAGS_NAME", tc.featureFlagEnvValue)
			}
			got := config.GetFeatureFlagsConfigName()
			want := tc.expected
			if got != want {
				t.Errorf("GetFeatureFlagsConfigName() = %s, want %s", got, want)
			}
		})
	}
}

func TestNewFeatureFlagsConfigMapErrors(t *testing.T) {
	for _, tc := range []struct {
		fileName string
	}{{
		fileName: "feature-flags-invalid-boolean",
	}, {
		fileName: "feature-flags-invalid-enable-api-fields",
	}, {
		fileName: "feature-flags-invalid-embedded-status",
	}} {
		t.Run(tc.fileName, func(t *testing.T) {
			cm := test.ConfigMapFromTestFile(t, tc.fileName)
			if _, err := config.NewFeatureFlagsFromConfigMap(cm); err == nil {
				t.Error("expected error but received nil")
			}
		})
	}
}

func verifyConfigFileWithExpectedFeatureFlagsConfig(t *testing.T, fileName string, expectedConfig *config.FeatureFlags) {
	cm := test.ConfigMapFromTestFile(t, fileName)
	if flags, err := config.NewFeatureFlagsFromConfigMap(cm); err == nil {
		if d := cmp.Diff(expectedConfig, flags); d != "" {
			t.Errorf("Diff:\n%s", diff.PrintWantGot(d))
		}
	} else {
		t.Errorf("NewFeatureFlagsFromConfigMap(actual) = %v", err)
	}
}
