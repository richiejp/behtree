package behtree_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/richiejp/behtree"
)

var _ = Describe("Condition", func() {
	Context("Resolve", func() {
		It("resolves literal object and value", func() {
			c := behtree.Condition{Object: "robot", Field: "location", Value: "table"}
			rc, err := c.Resolve(behtree.Params{})
			Expect(err).NotTo(HaveOccurred())
			Expect(rc.Object).To(Equal("robot"))
			Expect(rc.Field).To(Equal("location"))
			Expect(rc.Value).To(Equal("table"))
			Expect(rc.ValueRef).To(BeNil())
		})

		It("resolves $param in value to literal", func() {
			c := behtree.Condition{Object: "robot", Field: "holding", Value: "$object"}
			rc, err := c.Resolve(behtree.Params{"object": "wrapper"})
			Expect(err).NotTo(HaveOccurred())
			Expect(rc.Object).To(Equal("robot"))
			Expect(rc.Value).To(Equal("wrapper"))
			Expect(rc.ValueRef).To(BeNil())
		})

		It("resolves $param in object to literal", func() {
			c := behtree.Condition{Object: "$object", Field: "location", Value: "$container"}
			rc, err := c.Resolve(behtree.Params{"object": "wrapper", "container": "bin"})
			Expect(err).NotTo(HaveOccurred())
			Expect(rc.Object).To(Equal("wrapper"))
			Expect(rc.Value).To(Equal("bin"))
		})

		It("resolves $param.field in value to StateRef", func() {
			c := behtree.Condition{Object: "robot", Field: "location", Value: "$object.location"}
			rc, err := c.Resolve(behtree.Params{"object": "wrapper"})
			Expect(err).NotTo(HaveOccurred())
			Expect(rc.Object).To(Equal("robot"))
			Expect(rc.ValueRef).NotTo(BeNil())
			Expect(rc.ValueRef.Object).To(Equal("wrapper"))
			Expect(rc.ValueRef.Field).To(Equal("location"))
		})

		It("errors on missing param in object", func() {
			c := behtree.Condition{Object: "$missing", Field: "f", Value: "v"}
			_, err := c.Resolve(behtree.Params{})
			Expect(err).To(HaveOccurred())
		})

		It("errors on missing param in value", func() {
			c := behtree.Condition{Object: "o", Field: "f", Value: "$missing"}
			_, err := c.Resolve(behtree.Params{})
			Expect(err).To(HaveOccurred())
		})
	})

	Context("Evaluate", func() {
		var state *behtree.State

		BeforeEach(func() {
			state = behtree.NewState()
			state.Objects["robot"] = map[string]any{"location": "table", "holding": ""}
			state.Objects["wrapper"] = map[string]any{"location": "table"}
		})

		It("returns true when literal value matches", func() {
			rc := behtree.ResolvedCondition{Object: "robot", Field: "location", Value: "table"}
			result, err := rc.Evaluate(state)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeTrue())
		})

		It("returns false when literal value does not match", func() {
			rc := behtree.ResolvedCondition{Object: "robot", Field: "location", Value: "bin"}
			result, err := rc.Evaluate(state)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeFalse())
		})

		It("evaluates StateRef dynamically", func() {
			rc := behtree.ResolvedCondition{
				Object:   "robot",
				Field:    "location",
				ValueRef: &behtree.StateRef{Object: "wrapper", Field: "location"},
			}
			result, err := rc.Evaluate(state)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeTrue())

			// Move wrapper, condition should now fail
			Expect(state.Set("wrapper", "location", "bin")).To(Succeed())
			result, err = rc.Evaluate(state)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeFalse())
		})

		It("matches empty string for empty holding", func() {
			rc := behtree.ResolvedCondition{Object: "robot", Field: "holding", Value: ""}
			result, err := rc.Evaluate(state)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeTrue())
		})
	})

	Context("Matches", func() {
		It("matches identical literal conditions", func() {
			a := behtree.ResolvedCondition{Object: "robot", Field: "location", Value: "table"}
			b := behtree.ResolvedCondition{Object: "robot", Field: "location", Value: "table"}
			Expect(a.Matches(b)).To(BeTrue())
		})

		It("does not match different values", func() {
			a := behtree.ResolvedCondition{Object: "robot", Field: "location", Value: "table"}
			b := behtree.ResolvedCondition{Object: "robot", Field: "location", Value: "bin"}
			Expect(a.Matches(b)).To(BeFalse())
		})

		It("does not match different fields", func() {
			a := behtree.ResolvedCondition{Object: "robot", Field: "location", Value: "table"}
			b := behtree.ResolvedCondition{Object: "robot", Field: "holding", Value: "table"}
			Expect(a.Matches(b)).To(BeFalse())
		})

		It("matches identical StateRef conditions", func() {
			ref := &behtree.StateRef{Object: "wrapper", Field: "location"}
			a := behtree.ResolvedCondition{Object: "robot", Field: "location", ValueRef: ref}
			b := behtree.ResolvedCondition{Object: "robot", Field: "location", ValueRef: ref}
			Expect(a.Matches(b)).To(BeTrue())
		})

		It("does not match literal vs StateRef", func() {
			a := behtree.ResolvedCondition{Object: "robot", Field: "location", Value: "table"}
			b := behtree.ResolvedCondition{
				Object:   "robot",
				Field:    "location",
				ValueRef: &behtree.StateRef{Object: "wrapper", Field: "location"},
			}
			Expect(a.Matches(b)).To(BeFalse())
		})
	})

	Context("Contradicts", func() {
		It("contradicts different values on same field", func() {
			a := behtree.ResolvedCondition{Object: "robot", Field: "location", Value: "table"}
			b := behtree.ResolvedCondition{Object: "robot", Field: "location", Value: "bin"}
			Expect(a.Contradicts(b)).To(BeTrue())
		})

		It("does not contradict matching conditions", func() {
			a := behtree.ResolvedCondition{Object: "robot", Field: "location", Value: "table"}
			b := behtree.ResolvedCondition{Object: "robot", Field: "location", Value: "table"}
			Expect(a.Contradicts(b)).To(BeFalse())
		})

		It("does not contradict different fields", func() {
			a := behtree.ResolvedCondition{Object: "robot", Field: "location", Value: "table"}
			b := behtree.ResolvedCondition{Object: "robot", Field: "holding", Value: "table"}
			Expect(a.Contradicts(b)).To(BeFalse())
		})
	})

	Context("Name", func() {
		It("generates readable name for literal condition", func() {
			rc := behtree.ResolvedCondition{Object: "robot", Field: "location", Value: "table"}
			Expect(rc.Name()).To(Equal("robot.location==table"))
		})

		It("generates readable name for StateRef condition", func() {
			rc := behtree.ResolvedCondition{
				Object:   "robot",
				Field:    "location",
				ValueRef: &behtree.StateRef{Object: "wrapper", Field: "location"},
			}
			Expect(rc.Name()).To(Equal("robot.location==wrapper.location"))
		})
	})
})
