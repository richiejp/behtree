package behtree_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/richiejp/behtree"
	"github.com/richiejp/behtree/internal/benchcases"
)

var _ = Describe("PA-BT", func() {
	var (
		state   *behtree.State
		actions []behtree.GroundedAction
	)

	BeforeEach(func() {
		state = robotTestState()
		actions = robotTestActions()
	})

	Context("BuildTree", func() {
		It("builds a tree for wrapper.location==bin goal", func() {
			goal := []behtree.ResolvedCondition{
				{Object: "wrapper", Field: "location", Value: "bin"},
			}

			result, err := behtree.BuildTree(goal, actions, state)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Tree).NotTo(BeNil())
			Expect(result.Registry).NotTo(BeNil())

			// The tree should be printable
			output := behtree.PrintTree(result.Tree)
			Expect(output).To(ContainSubstring("Sequence"))
			Expect(output).To(ContainSubstring("Fallback"))
			Expect(output).To(ContainSubstring("DropIn"))
		})

		It("produces a tree that succeeds with action handlers", func() {
			goal := []behtree.ResolvedCondition{
				{Object: "wrapper", Field: "location", Value: "bin"},
			}

			result, err := behtree.BuildTree(goal, actions, state)
			Expect(err).NotTo(HaveOccurred())

			// Register actual action handlers
			benchcases.RegisterRobotHandlers(result.Registry)

			ip := behtree.NewInterpreter(nil, result.Registry, state)
			ip.SetOutcomeRequest(behtree.RequestSuccess)

			var finalStatus behtree.Status
			for range 20 {
				status, tickErr := ip.Tick(result.Tree)
				Expect(tickErr).NotTo(HaveOccurred())
				finalStatus = status
				if status == behtree.Success {
					break
				}
			}
			Expect(finalStatus).To(Equal(behtree.Success))

			// Verify final state
			wrapperLoc, _ := state.Get("wrapper", "location")
			Expect(wrapperLoc).To(Equal("bin"))

			holding, _ := state.Get("robot", "holding")
			Expect(holding).To(Equal(""))
		})

		It("errors on empty goal", func() {
			_, err := behtree.BuildTree(nil, actions, state)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("empty goal"))
		})

		It("errors when no action satisfies a condition", func() {
			goal := []behtree.ResolvedCondition{
				{Object: "robot", Field: "flying", Value: "true"},
			}
			_, err := behtree.BuildTree(goal, actions, state)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot satisfy"))
		})

		It("handles goals already satisfied in initial state", func() {
			// Set wrapper already at bin
			Expect(state.Set("wrapper", "location", "bin")).To(Succeed())

			goal := []behtree.ResolvedCondition{
				{Object: "wrapper", Field: "location", Value: "bin"},
			}

			result, err := behtree.BuildTree(goal, actions, state)
			Expect(err).NotTo(HaveOccurred())

			// Tree should be just the condition check (no expansions needed)
			Expect(result.Tree.Type).To(Equal(behtree.SequenceNode))
			Expect(result.Tree.Children).To(HaveLen(1))
			Expect(result.Tree.Children[0].Type).To(Equal(behtree.ConditionNode))
		})

		It("handles StateRef preconditions correctly", func() {
			goal := []behtree.ResolvedCondition{
				{Object: "robot", Field: "holding", Value: "wrapper"},
			}

			result, err := behtree.BuildTree(goal, actions, state)
			Expect(err).NotTo(HaveOccurred())

			// The tree should contain a condition with a StateRef
			// (robot.location==wrapper.location)
			output := behtree.PrintTree(result.Tree)
			Expect(output).To(ContainSubstring("PickUp"))
			Expect(output).To(ContainSubstring("robot.location==wrapper.location"))
		})
	})

	Context("Desktop scenario", func() {
		It("builds a tree for opening a URL in Firefox", func() {
			dState := desktopTestState()
			dActions := desktopTestActions()
			dGoal := desktopTestGoal()

			result, err := behtree.BuildTree(dGoal, dActions, dState)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Tree).NotTo(BeNil())

			output := behtree.PrintTree(result.Tree)
			Expect(output).To(ContainSubstring("QuerySwayTree"))
			Expect(output).To(ContainSubstring("OpenApp"))
			Expect(output).To(ContainSubstring("OpenURL"))
			Expect(output).To(ContainSubstring("firefox.active_page==" + desktopTestURL))
			Expect(output).To(ContainSubstring("firefox.open==true"))
			Expect(output).To(ContainSubstring("sway_state.refreshed==true"))
		})

		It("produces a tree that succeeds with action handlers", func() {
			dState := desktopTestState()
			dActions := desktopTestActions()
			dGoal := desktopTestGoal()

			result, err := behtree.BuildTree(dGoal, dActions, dState)
			Expect(err).NotTo(HaveOccurred())

			benchcases.RegisterDesktopInnerHandlers(result.Registry)

			ip := behtree.NewInterpreter(nil, result.Registry, dState)
			ip.SetOutcomeRequest(behtree.RequestSuccess)

			var finalStatus behtree.Status
			for range 20 {
				status, tickErr := ip.Tick(result.Tree)
				Expect(tickErr).NotTo(HaveOccurred())
				finalStatus = status
				if status == behtree.Success {
					break
				}
			}
			Expect(finalStatus).To(Equal(behtree.Success))

			activePage, _ := dState.Get("firefox", "active_page")
			Expect(activePage).To(Equal(desktopTestURL))

			open, _ := dState.Get("firefox", "open")
			Expect(open).To(Equal("true"))

			refreshed, _ := dState.Get("sway_state", "refreshed")
			Expect(refreshed).To(Equal("true"))
		})

		It("handles goal already satisfied", func() {
			dState := desktopTestState()
			Expect(dState.Set("firefox", "active_page", desktopTestURL)).To(Succeed())

			dActions := desktopTestActions()
			dGoal := desktopTestGoal()

			result, err := behtree.BuildTree(dGoal, dActions, dState)
			Expect(err).NotTo(HaveOccurred())

			Expect(result.Tree.Type).To(Equal(behtree.SequenceNode))
			Expect(result.Tree.Children).To(HaveLen(1))
			Expect(result.Tree.Children[0].Type).To(Equal(behtree.ConditionNode))
		})
	})
})
