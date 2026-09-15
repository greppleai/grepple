package aiprovider

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"charm.land/fantasy"
	fantasybedrock "charm.land/fantasy/providers/bedrock"
	"github.com/aws/aws-sdk-go-v2/config"
)

const bedrockProviderName = "bedrock"

type bedrockProvider struct {
	store *Store
}

type bedrockCredentials struct {
	ValidatedAt time.Time `json:"validatedAt"`
}

func newBedrockProvider(store *Store, _ *http.Client) *bedrockProvider {
	return &bedrockProvider{store: store}
}

func (p *bedrockProvider) Name() string { return bedrockProviderName }

func (p *bedrockProvider) DefaultModel() string {
	return "anthropic.claude-sonnet-4-5-20250929-v1:0"
}

func (p *bedrockProvider) Login(ctx context.Context, _ LoginOptions) error {
	if err := validateAWSCredentials(ctx); err != nil {
		return err
	}
	return p.store.Save(p.Name(), bedrockCredentials{ValidatedAt: time.Now().UTC()})
}

func (p *bedrockProvider) Logout() error {
	return p.store.Delete(p.Name())
}

func (p *bedrockProvider) LoggedIn() (bool, error) {
	var credentials bedrockCredentials
	found, err := p.store.Load(p.Name(), &credentials)
	if err != nil || !found || credentials.ValidatedAt.IsZero() {
		return false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := validateAWSCredentials(ctx); err != nil {
		return false, nil
	}
	return true, nil
}

func (p *bedrockProvider) LanguageModel(ctx context.Context, modelID string) (fantasy.LanguageModel, error) {
	if err := validateAWSCredentials(ctx); err != nil {
		return nil, err
	}
	provider, err := fantasybedrock.New()
	if err != nil {
		return nil, err
	}
	return provider.LanguageModel(ctx, modelID)
}

func validateAWSCredentials(ctx context.Context) error {
	if strings.TrimSpace(os.Getenv("AWS_REGION")) == "" {
		return fmt.Errorf("AWS_REGION is required for Bedrock model routing")
	}
	configuration, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("load AWS configuration for Bedrock: %w", err)
	}
	credentials, err := configuration.Credentials.Retrieve(ctx)
	if err != nil {
		return fmt.Errorf("resolve AWS credentials for Bedrock: %w", err)
	}
	if credentials.AccessKeyID == "" {
		return fmt.Errorf("resolve AWS credentials for Bedrock: empty access key")
	}
	return nil
}
