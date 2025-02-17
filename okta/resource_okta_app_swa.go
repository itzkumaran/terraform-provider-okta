package okta

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/okta/okta-sdk-golang/v2/okta"
	"github.com/okta/okta-sdk-golang/v2/okta/query"
)

func resourceAppSwa() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceAppSwaCreate,
		ReadContext:   resourceAppSwaRead,
		UpdateContext: resourceAppSwaUpdate,
		DeleteContext: resourceAppSwaDelete,
		Importer: &schema.ResourceImporter{
			StateContext: appImporter,
		},
		Schema: buildAppSwaSchema(map[string]*schema.Schema{
			"preconfigured_app": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Preconfigured app name",
			},
			"button_field": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Login button field",
			},
			"password_field": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Login password field",
			},
			"username_field": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Login username field",
			},
			"url": {
				Type:             schema.TypeString,
				Optional:         true,
				Description:      "Login URL",
				ValidateDiagFunc: stringIsURL(validURLSchemes...),
			},
			"url_regex": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "A regex that further restricts URL to the specified regex",
			},
			"checkbox": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "CSS selector for the checkbox",
			},
			"redirect_url": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "If going to the login page URL redirects to another page, then enter that URL here",
			},
		}),
		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(1 * time.Hour),
			Read:   schema.DefaultTimeout(1 * time.Hour),
			Update: schema.DefaultTimeout(1 * time.Hour),
		},
	}
}

func resourceAppSwaCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := getOktaClientFromMetadata(m)
	app := buildAppSwa(d)
	activate := d.Get("status").(string) == statusActive
	params := &query.Params{Activate: &activate}
	_, _, err := client.Application.CreateApplication(ctx, app, params)
	if err != nil {
		return diag.Errorf("failed to create SWA application: %v", err)
	}
	d.SetId(app.Id)
	err = handleAppGroupsAndUsers(ctx, app.Id, d, m)
	if err != nil {
		return diag.Errorf("failed to handle groups and users for SWA application: %v", err)
	}
	err = handleAppLogo(ctx, d, m, app.Id, app.Links)
	if err != nil {
		return diag.Errorf("failed to upload logo for SWA application: %v", err)
	}
	return resourceAppSwaRead(ctx, d, m)
}

func resourceAppSwaRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	app := okta.NewSwaApplication()
	err := fetchApp(ctx, d, m, app)
	if err != nil {
		return diag.Errorf("failed to get SWA application: %v", err)
	}
	if app.Id == "" {
		d.SetId("")
		return nil
	}
	_ = d.Set("button_field", app.Settings.App.ButtonField)
	_ = d.Set("password_field", app.Settings.App.PasswordField)
	_ = d.Set("username_field", app.Settings.App.UsernameField)
	_ = d.Set("url", app.Settings.App.Url)
	_ = d.Set("url_regex", app.Settings.App.LoginUrlRegex)
	_ = d.Set("checkbox", app.Settings.App.Checkbox)
	_ = d.Set("redirect_url", app.Settings.App.RedirectUrl)
	_ = d.Set("user_name_template", app.Credentials.UserNameTemplate.Template)
	_ = d.Set("user_name_template_type", app.Credentials.UserNameTemplate.Type)
	_ = d.Set("user_name_template_suffix", app.Credentials.UserNameTemplate.Suffix)
	_ = d.Set("user_name_template_push_status", app.Credentials.UserNameTemplate.PushStatus)
	_ = d.Set("logo_url", linksValue(app.Links, "logo", "href"))
	appRead(d, app.Name, app.Status, app.SignOnMode, app.Label, app.Accessibility, app.Visibility, app.Settings.Notes)
	err = syncGroupsAndUsers(ctx, app.Id, d, m)
	if err != nil {
		return diag.Errorf("failed to sync groups and users for SWA application: %v", err)
	}
	return nil
}

func resourceAppSwaUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := getOktaClientFromMetadata(m)
	app := buildAppSwa(d)
	_, _, err := client.Application.UpdateApplication(ctx, d.Id(), app)
	if err != nil {
		return diag.Errorf("failed to update SWA application: %v", err)
	}
	err = setAppStatus(ctx, d, client, app.Status)
	if err != nil {
		return diag.Errorf("failed to set SWA application status: %v", err)
	}
	err = handleAppGroupsAndUsers(ctx, app.Id, d, m)
	if err != nil {
		return diag.Errorf("failed to handle groups and users for SWA application: %v", err)
	}
	if d.HasChange("logo") {
		err = handleAppLogo(ctx, d, m, app.Id, app.Links)
		if err != nil {
			o, _ := d.GetChange("logo")
			_ = d.Set("logo", o)
			return diag.Errorf("failed to upload logo for SWA application: %v", err)
		}
	}
	return resourceAppSwaRead(ctx, d, m)
}

func resourceAppSwaDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	err := deleteApplication(ctx, d, m)
	if err != nil {
		return diag.Errorf("failed to delete SWA application: %v", err)
	}
	return nil
}

func buildAppSwa(d *schema.ResourceData) *okta.SwaApplication {
	// Abstracts away name and SignOnMode which are constant for this app type.
	app := okta.NewSwaApplication()
	app.Label = d.Get("label").(string)
	name := d.Get("preconfigured_app").(string)
	if name != "" {
		app.Name = name
		app.SignOnMode = "AUTO_LOGIN" // in case pre-configured app has more than one sign-on modes
	}
	app.Settings = &okta.SwaApplicationSettings{
		App: &okta.SwaApplicationSettingsApplication{
			ButtonField:   d.Get("button_field").(string),
			UsernameField: d.Get("username_field").(string),
			PasswordField: d.Get("password_field").(string),
			Url:           d.Get("url").(string),
			LoginUrlRegex: d.Get("url_regex").(string),
			RedirectUrl:   d.Get("redirect_url").(string),
			Checkbox:      d.Get("checkbox").(string),
		},
		Notes: buildAppNotes(d),
	}
	app.Visibility = buildAppVisibility(d)
	app.Accessibility = buildAppAccessibility(d)
	app.Credentials = &okta.SchemeApplicationCredentials{
		UserNameTemplate: buildUserNameTemplate(d),
	}
	return app
}
