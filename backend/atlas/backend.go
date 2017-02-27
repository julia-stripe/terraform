package atlas

import (
	"context"
	"fmt"
	"sync"

	"github.com/hashicorp/terraform/backend"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/hashicorp/terraform/state"
	"github.com/hashicorp/terraform/terraform"
	"github.com/mitchellh/cli"
	"github.com/mitchellh/colorstring"
)

// Backend is an implementation of EnhancedBackend that performs all operations
// in Atlas. State must currently also be stored in Atlas, although it is worth
// investigating in the future if state storage can be external as well.
type Backend struct {
	// CLI and Colorize control the CLI output. If CLI is nil then no CLI
	// output will be done. If CLIColor is nil then no coloring will be done.
	CLI      cli.Ui
	CLIColor *colorstring.Colorize

	// ContextOpts are the base context options to set when initializing a
	// Terraform context. Many of these will be overridden or merged by
	// Operation. See Operation for more details.
	ContextOpts *terraform.ContextOpts

	schema *schema.Backend
	opLock sync.Mutex
	once   sync.Once
}

func (b *Backend) Input(
	ui terraform.UIInput, c *terraform.ResourceConfig) (*terraform.ResourceConfig, error) {
	b.once.Do(b.init)
	return b.schema.Input(ui, c)
}

func (b *Backend) Validate(c *terraform.ResourceConfig) ([]string, []error) {
	b.once.Do(b.init)
	return b.schema.Validate(c)
}

func (b *Backend) Configure(c *terraform.ResourceConfig) error {
	b.once.Do(b.init)
	return b.schema.Configure(c)
}

func (b *Backend) State() (state.State, error) {
	return nil, nil
}

// Operation implements backend.Enhanced
//
// This will initialize an in-memory terraform.Context to perform the
// operation within this process.
//
// The given operation parameter will be merged with the ContextOpts on
// the structure with the following rules. If a rule isn't specified and the
// name conflicts, assume that the field is overwritten if set.
func (b *Backend) Operation(ctx context.Context, op *backend.Operation) (*backend.RunningOperation, error) {
	// Determine the function to call for our operation
	var f func(context.Context, *backend.Operation, *backend.RunningOperation)
	switch op.Type {
	/*
		case backend.OperationTypeRefresh:
			f = b.opRefresh
		case backend.OperationTypePlan:
			f = b.opPlan
		case backend.OperationTypeApply:
			f = b.opApply
	*/
	default:
		return nil, fmt.Errorf(
			"Unsupported operation type: %s\n\n"+
				"This is a bug in Terraform and should be reported.",
			op.Type)
	}

	// Lock
	b.opLock.Lock()

	// Build our running operation
	runningCtx, runningCtxCancel := context.WithCancel(context.Background())
	runningOp := &backend.RunningOperation{Context: runningCtx}

	// Do it
	go func() {
		defer b.opLock.Unlock()
		defer runningCtxCancel()
		f(ctx, op, runningOp)
	}()

	// Return
	return runningOp, nil
}

// Colorize returns the Colorize structure that can be used for colorizing
// output. This is gauranteed to always return a non-nil value and so is useful
// as a helper to wrap any potentially colored strings.
func (b *Backend) Colorize() *colorstring.Colorize {
	if b.CLIColor != nil {
		return b.CLIColor
	}

	return &colorstring.Colorize{
		Colors:  colorstring.DefaultColors,
		Disable: true,
	}
}

func (b *Backend) init() {
	b.schema = &schema.Backend{
		Schema: map[string]*schema.Schema{
			"name": &schema.Schema{
				Type:        schema.TypeString,
				Required:    true,
				Description: schemaDescriptions["name"],
			},

			"access_token": &schema.Schema{
				Type:        schema.TypeString,
				Required:    true,
				Description: schemaDescriptions["access_token"],
				DefaultFunc: schema.EnvDefaultFunc("ATLAS_TOKEN", nil),
			},

			"address": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				Description: schemaDescriptions["address"],
			},
		},

		ConfigureFunc: b.schemaConfigure,
	}
}

func (b *Backend) schemaConfigure(ctx context.Context) error {
	/*
		d := schema.FromContextBackendConfig(ctx)

		// Set the path if it is set
		pathRaw, ok := d.GetOk("path")
		if ok {
			path := pathRaw.(string)
			if path == "" {
				return fmt.Errorf("configured path is empty")
			}

			b.StatePath = path
		}
	*/

	return nil
}

var schemaDescriptions = map[string]string{
	"name": "Full name of the environment in Atlas, such as 'hashicorp/myenv'",
	"access_token": "Access token to use to access Atlas. If ATLAS_TOKEN is set then\n" +
		"this will override any saved value for this.",
	"address": "Address to your Atlas installation. This defaults to the publicly\n" +
		"hosted version at 'https://atlas.hashicorp.com/'. This address\n" +
		"should contain the full HTTP scheme to use.",
}
