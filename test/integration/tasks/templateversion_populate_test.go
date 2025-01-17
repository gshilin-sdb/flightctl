package tasks_test

import (
	"context"

	api "github.com/flightctl/flightctl/api/v1alpha1"
	"github.com/flightctl/flightctl/internal/config"
	"github.com/flightctl/flightctl/internal/store"
	"github.com/flightctl/flightctl/internal/store/model"
	"github.com/flightctl/flightctl/internal/tasks"
	"github.com/flightctl/flightctl/internal/util"
	flightlog "github.com/flightctl/flightctl/pkg/log"
	"github.com/flightctl/flightctl/pkg/queues"
	testutil "github.com/flightctl/flightctl/test/util"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	"go.uber.org/mock/gomock"
)

var _ = Describe("TVPopulate", func() {
	var (
		log             *logrus.Logger
		ctx             context.Context
		orgId           uuid.UUID
		storeInst       store.Store
		cfg             *config.Config
		dbName          string
		callbackManager tasks.CallbackManager
		fleet           *api.Fleet
		tv              *api.TemplateVersion
		fleetCallback   store.FleetStoreCallback
		ctrl            *gomock.Controller
		publisher       *queues.MockPublisher
	)

	BeforeEach(func() {
		ctx = context.Background()
		orgId, _ = uuid.NewUUID()
		log = flightlog.InitLogs()
		storeInst, cfg, dbName, _ = store.PrepareDBForUnitTests(log)
		ctrl = gomock.NewController(GinkgoT())
		publisher = queues.NewMockPublisher(ctrl)
		publisher.EXPECT().Publish(gomock.Any()).AnyTimes()
		callbackManager = tasks.NewCallbackManager(publisher, log)
		fleetCallback = func(before *model.Fleet, after *model.Fleet) {}

		fleet = &api.Fleet{
			Metadata: api.ObjectMeta{Name: util.StrToPtr("fleet")},
		}
		_, err := storeInst.Fleet().Create(ctx, orgId, fleet, fleetCallback)
		Expect(err).ToNot(HaveOccurred())

		testutil.CreateTestDevices(ctx, 2, storeInst.Device(), orgId, util.SetResourceOwner(model.FleetKind, *fleet.Metadata.Name), false)

		tv = &api.TemplateVersion{
			Metadata: api.ObjectMeta{
				Name:  util.StrToPtr("tv"),
				Owner: util.SetResourceOwner(model.FleetKind, *fleet.Metadata.Name),
			},
			Spec: api.TemplateVersionSpec{Fleet: *fleet.Metadata.Name},
		}
		tvCallback := store.TemplateVersionStoreCallback(func(tv *model.TemplateVersion) {})
		_, err = storeInst.TemplateVersion().Create(ctx, orgId, tv, tvCallback)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		ctrl.Finish()
		store.DeleteTestDB(log, cfg, storeInst, dbName)
	})

	When("a template has a valid inline config with no params", func() {
		It("copies the config as is", func() {
			inlineConfig := &api.InlineConfigProviderSpec{
				Name: "inlineConfig",
			}
			base64 := api.Base64
			inlineConfig.Inline = []api.FileSpec{
				{Path: "/etc/base64encoded", Content: "SGVsbG8gd29ybGQsIHdoYXQncyB1cD8=", ContentEncoding: &base64},
				{Path: "/etc/notencoded", Content: "Hello world, what's up?"},
			}
			inlineItem := api.ConfigProviderSpec{}
			err := inlineItem.FromInlineConfigProviderSpec(*inlineConfig)
			Expect(err).ToNot(HaveOccurred())

			fleet.Spec.Template.Spec.Config = &[]api.ConfigProviderSpec{inlineItem}
			_, _, err = storeInst.Fleet().CreateOrUpdate(ctx, orgId, fleet, fleetCallback)
			Expect(err).ToNot(HaveOccurred())

			owner := util.SetResourceOwner(model.FleetKind, *fleet.Metadata.Name)
			resourceRef := tasks.ResourceReference{OrgID: orgId, Op: tasks.TemplateVersionPopulateOpCreated, Name: "tv", Kind: model.TemplateVersionKind, Owner: *owner}
			logic := tasks.NewTemplateVersionPopulateLogic(callbackManager, log, storeInst, nil, resourceRef)
			err = logic.SyncFleetTemplateToTemplateVersion(ctx)
			Expect(err).ToNot(HaveOccurred())

			tv, err = storeInst.TemplateVersion().Get(ctx, orgId, *fleet.Metadata.Name, *tv.Metadata.Name)
			Expect(err).ToNot(HaveOccurred())

			Expect(tv.Status.Config).ToNot(BeNil())
			Expect(*tv.Status.Config).To(HaveLen(1))
			configItem := (*tv.Status.Config)[0]
			newInline, err := configItem.AsInlineConfigProviderSpec()
			Expect(err).ToNot(HaveOccurred())

			Expect(newInline.Inline[0].Content).To(Equal("SGVsbG8gd29ybGQsIHdoYXQncyB1cD8="))
			Expect(newInline.Inline[1].Content).To(Equal("Hello world, what's up?"))
		})
	})

	When("a template has a valid inline config with params", func() {
		It("copies the config as is", func() {
			inlineConfig := &api.InlineConfigProviderSpec{
				Name: "inlineConfig",
			}
			base64 := api.Base64
			inlineConfig.Inline = []api.FileSpec{
				// Unencoded: I have a parameter {{ device.metadata.labels[key] }}
				{Path: "/etc/base64encoded", Content: "SSBoYXZlIGEgcGFyYW1ldGVyIHt7IGRldmljZS5tZXRhZGF0YS5sYWJlbHNba2V5XSB9fQ==", ContentEncoding: &base64},
				{Path: "/etc/urlencoded", Content: "I have a parameter {{ device.metadata.labels[key] }}"},
			}

			inlineItem := api.ConfigProviderSpec{}
			err := inlineItem.FromInlineConfigProviderSpec(*inlineConfig)
			Expect(err).ToNot(HaveOccurred())

			fleet.Spec.Template.Spec.Config = &[]api.ConfigProviderSpec{inlineItem}
			_, _, err = storeInst.Fleet().CreateOrUpdate(ctx, orgId, fleet, fleetCallback)
			Expect(err).ToNot(HaveOccurred())

			owner := util.SetResourceOwner(model.FleetKind, *fleet.Metadata.Name)
			resourceRef := tasks.ResourceReference{OrgID: orgId, Op: tasks.TemplateVersionPopulateOpCreated, Name: "tv", Kind: model.TemplateVersionKind, Owner: *owner}
			logic := tasks.NewTemplateVersionPopulateLogic(callbackManager, log, storeInst, nil, resourceRef)
			err = logic.SyncFleetTemplateToTemplateVersion(ctx)
			Expect(err).ToNot(HaveOccurred())

			tv, err = storeInst.TemplateVersion().Get(ctx, orgId, *fleet.Metadata.Name, *tv.Metadata.Name)
			Expect(err).ToNot(HaveOccurred())

			Expect(tv.Status.Config).ToNot(BeNil())
			Expect(*tv.Status.Config).To(HaveLen(1))
			configItem := (*tv.Status.Config)[0]
			newInline, err := configItem.AsInlineConfigProviderSpec()
			Expect(err).ToNot(HaveOccurred())

			Expect(newInline.Inline[0].Content).To(Equal("SSBoYXZlIGEgcGFyYW1ldGVyIHt7IGRldmljZS5tZXRhZGF0YS5sYWJlbHNba2V5XSB9fQ=="))
			Expect(newInline.Inline[1].Content).To(Equal("I have a parameter {{ device.metadata.labels[key] }}"))
		})
	})

	When("a template has a valid HTTP config with params", func() {
		It("copies the config as is", func() {
			httpConfig := &api.HttpConfigProviderSpec{
				Name: "httpConfig",
			}
			httpConfig.HttpRef.Repository = "repo"
			httpConfig.HttpRef.FilePath = "filepath-{{ device.metadata.name }}"
			httpConfig.HttpRef.Suffix = util.StrToPtr("suffix")

			httpItem := api.ConfigProviderSpec{}
			err := httpItem.FromHttpConfigProviderSpec(*httpConfig)
			Expect(err).ToNot(HaveOccurred())

			fleet.Spec.Template.Spec.Config = &[]api.ConfigProviderSpec{httpItem}
			_, _, err = storeInst.Fleet().CreateOrUpdate(ctx, orgId, fleet, fleetCallback)
			Expect(err).ToNot(HaveOccurred())

			owner := util.SetResourceOwner(model.FleetKind, *fleet.Metadata.Name)
			resourceRef := tasks.ResourceReference{OrgID: orgId, Op: tasks.TemplateVersionPopulateOpCreated, Name: "tv", Kind: model.TemplateVersionKind, Owner: *owner}
			logic := tasks.NewTemplateVersionPopulateLogic(callbackManager, log, storeInst, nil, resourceRef)
			err = logic.SyncFleetTemplateToTemplateVersion(ctx)
			Expect(err).ToNot(HaveOccurred())

			tv, err = storeInst.TemplateVersion().Get(ctx, orgId, *fleet.Metadata.Name, *tv.Metadata.Name)
			Expect(err).ToNot(HaveOccurred())

			Expect(tv.Status.Config).ToNot(BeNil())
			Expect(*tv.Status.Config).To(HaveLen(1))
			configItem := (*tv.Status.Config)[0]
			newHttp, err := configItem.AsHttpConfigProviderSpec()
			Expect(err).ToNot(HaveOccurred())

			Expect(newHttp.Name).To(Equal(httpConfig.Name))
			Expect(newHttp.HttpRef.Repository).To(Equal(httpConfig.HttpRef.Repository))
			Expect(newHttp.HttpRef.FilePath).To(Equal(httpConfig.HttpRef.FilePath))
			Expect(*newHttp.HttpRef.Suffix).To(Equal(*httpConfig.HttpRef.Suffix))
		})
	})
})
