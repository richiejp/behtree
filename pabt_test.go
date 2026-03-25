package behtree_test

import (
	"strings"

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
			Expect(output).To(ContainSubstring("Observe"))
			Expect(output).To(ContainSubstring("firefox.active_page==" + desktopTestURL))
			Expect(output).To(ContainSubstring("firefox.open==true"))
			Expect(output).To(ContainSubstring("firefox.observed==true"))
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

			observed, _ := dState.Get("firefox", "observed")
			Expect(observed).To(Equal("true"))

			refreshed, _ := dState.Get("sway_state", "refreshed")
			Expect(refreshed).To(Equal("true"))
		})

		It("handles goal already satisfied", func() {
			dState := desktopTestState()
			Expect(dState.Set("firefox", "active_page", desktopTestURL)).To(Succeed())
			Expect(dState.Set("firefox", "observed", "true")).To(Succeed())

			dActions := desktopTestActions()
			dGoal := desktopTestGoal()

			result, err := behtree.BuildTree(dGoal, dActions, dState)
			Expect(err).NotTo(HaveOccurred())

			Expect(result.Tree.Type).To(Equal(behtree.SequenceNode))
			Expect(result.Tree.Children).To(HaveLen(2))
			Expect(result.Tree.Children[0].Type).To(Equal(behtree.ConditionNode))
			Expect(result.Tree.Children[1].Type).To(Equal(behtree.ConditionNode))
		})

		It("short-circuits when goal already satisfied (only Observe runs)", func() {
			dState := desktopTestState()
			// Set state so goal is already satisfied
			Expect(dState.Set("firefox", "active_page", desktopTestURL)).To(Succeed())
			Expect(dState.Set("firefox", "observed", "true")).To(Succeed())
			Expect(dState.Set("firefox", "open", "true")).To(Succeed())

			dActions := desktopTestActions()
			dGoal := desktopTestGoal()

			// Build from worst-case state so all fallbacks are expanded
			buildState := desktopTestState()
			result, err := behtree.BuildTree(dGoal, dActions, buildState)
			Expect(err).NotTo(HaveOccurred())

			// Track which action handlers are called
			called := map[string]int{}
			result.Registry.Register("Observe", func(params behtree.Params, s *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
				called["Observe"]++
				_ = s.Set(params["target"].(string), "observed", "true")
				return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
			})
			result.Registry.Register("QuerySwayTree", func(_ behtree.Params, s *behtree.State, _ behtree.OutcomeRequest) behtree.HandlerResult {
				called["QuerySwayTree"]++
				_ = s.Set("sway_state", "refreshed", "true")
				return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
			})
			result.Registry.Register("OpenApp", func(params behtree.Params, s *behtree.State, _ behtree.OutcomeRequest) behtree.HandlerResult {
				called["OpenApp"]++
				_ = s.Set(params["app"].(string), "open", "true")
				return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
			})
			result.Registry.Register("OpenURL", func(params behtree.Params, s *behtree.State, _ behtree.OutcomeRequest) behtree.HandlerResult {
				called["OpenURL"]++
				_ = s.Set(params["app"].(string), "active_page", params["url"].(string))
				return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
			})

			ip := behtree.NewInterpreter(nil, result.Registry, dState)
			ip.SetOutcomeRequest(behtree.RequestSuccess)

			status, tickErr := ip.Tick(result.Tree)
			Expect(tickErr).NotTo(HaveOccurred())
			Expect(status).To(Equal(behtree.Success))

			// Observe must run (ephemeral reset clears observed each tick)
			Expect(called["Observe"]).To(BeNumerically(">", 0))
			// No other actions should have run — goal was already satisfied
			Expect(called).NotTo(HaveKey("QuerySwayTree"))
			Expect(called).NotTo(HaveKey("OpenApp"))
			Expect(called).NotTo(HaveKey("OpenURL"))
		})
	})

	Context("Desktop outer scenario", func() {
		It("builds an outer tree with HandleSpeech, RunTaskTree, and Idle", func() {
			oState := desktopOuterTestState()
			oActions := desktopOuterTestActions()
			oGoal := desktopOuterTestGoal()

			result, err := behtree.BuildTree(oGoal, oActions, oState)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Tree).NotTo(BeNil())

			output := behtree.PrintTree(result.Tree)
			Expect(output).To(ContainSubstring("HandleSpeech"))
			Expect(output).To(ContainSubstring("RunTaskTree"))
			Expect(output).To(ContainSubstring("Idle"))
			Expect(output).To(ContainSubstring("system.idle==true"))
			Expect(output).To(ContainSubstring("speech.active==false"))
			Expect(output).To(ContainSubstring("task_tree.pending==false"))
		})

		It("checks speech before running tasks", func() {
			oState := desktopOuterTestState()
			oActions := desktopOuterTestActions()
			oGoal := desktopOuterTestGoal()

			result, err := behtree.BuildTree(oGoal, oActions, oState)
			Expect(err).NotTo(HaveOccurred())

			// The tree should have speech.active==false as a precondition
			// of RunTaskTree, verified by a second HandleSpeech fallback
			output := behtree.PrintTree(result.Tree)

			// HandleSpeech should appear twice:
			// once for top-level speech handling, once as RunTaskTree precondition
			Expect(strings.Count(output, "HandleSpeech")).To(Equal(2))
		})

		It("produces a tree that reaches idle when no speech or tasks", func() {
			// Build from worst-case state
			buildState := desktopOuterTestState()
			oActions := desktopOuterTestActions()
			oGoal := desktopOuterTestGoal()

			result, err := behtree.BuildTree(oGoal, oActions, buildState)
			Expect(err).NotTo(HaveOccurred())

			benchcases.RegisterDesktopOuterHandlers(result.Registry)

			// Runtime state: already idle (no speech, no tasks)
			runState := desktopOuterTestState()
			Expect(runState.Set("speech", "active", "false")).To(Succeed())
			Expect(runState.Set("task_tree", "pending", "false")).To(Succeed())

			ip := behtree.NewInterpreter(nil, result.Registry, runState)
			ip.SetOutcomeRequest(behtree.RequestSuccess)

			status, tickErr := ip.Tick(result.Tree)
			Expect(tickErr).NotTo(HaveOccurred())
			Expect(status).To(Equal(behtree.Success))

			idle, _ := runState.Get("system", "idle")
			Expect(idle).To(Equal("true"))
		})

		It("handles speech then reaches idle", func() {
			oState := desktopOuterTestState()
			// Speech active, no task
			Expect(oState.Set("speech", "active", "true")).To(Succeed())
			Expect(oState.Set("task_tree", "pending", "false")).To(Succeed())

			oActions := desktopOuterTestActions()
			oGoal := desktopOuterTestGoal()

			result, err := behtree.BuildTree(oGoal, oActions, oState)
			Expect(err).NotTo(HaveOccurred())

			benchcases.RegisterDesktopOuterHandlers(result.Registry)

			ip := behtree.NewInterpreter(nil, result.Registry, oState)
			ip.SetOutcomeRequest(behtree.RequestSuccess)

			status, tickErr := ip.Tick(result.Tree)
			Expect(tickErr).NotTo(HaveOccurred())
			Expect(status).To(Equal(behtree.Success))

			speech, _ := oState.Get("speech", "active")
			Expect(speech).To(Equal("false"))

			idle, _ := oState.Get("system", "idle")
			Expect(idle).To(Equal("true"))
		})

		It("handles speech then runs task tree then reaches idle", func() {
			oState := desktopOuterTestState()
			// Speech active, task pending with a simple subtree
			Expect(oState.Set("speech", "active", "true")).To(Succeed())
			Expect(oState.Set("task_tree", "pending", "true")).To(Succeed())

			// Set up a subtree that marks the task as done
			subtree := &behtree.Node{
				Type: behtree.ActionNode,
				Name: "CompleteTask",
			}
			Expect(oState.Set("task_tree", "tree", subtree)).To(Succeed())

			oActions := desktopOuterTestActions()
			oGoal := desktopOuterTestGoal()

			result, err := behtree.BuildTree(oGoal, oActions, oState)
			Expect(err).NotTo(HaveOccurred())

			benchcases.RegisterDesktopOuterHandlers(result.Registry)
			// Register the subtree's action handler
			result.Registry.Register("CompleteTask", func(_ behtree.Params, s *behtree.State, _ behtree.OutcomeRequest) behtree.HandlerResult {
				_ = s.Set("task_tree", "pending", "false")
				return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
			})

			ip := behtree.NewInterpreter(nil, result.Registry, oState)
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

			speech, _ := oState.Get("speech", "active")
			Expect(speech).To(Equal("false"))

			pending, _ := oState.Get("task_tree", "pending")
			Expect(pending).To(Equal("false"))

			idle, _ := oState.Get("system", "idle")
			Expect(idle).To(Equal("true"))
		})

		It("resets idle between ticks", func() {
			// Build tree from worst-case state so all fallbacks are expanded
			buildState := desktopOuterTestState()
			oActions := desktopOuterTestActions()
			oGoal := desktopOuterTestGoal()

			result, err := behtree.BuildTree(oGoal, oActions, buildState)
			Expect(err).NotTo(HaveOccurred())

			benchcases.RegisterDesktopOuterHandlers(result.Registry)

			// Runtime state: start idle (no speech, no tasks)
			runState := desktopOuterTestState()
			Expect(runState.Set("speech", "active", "false")).To(Succeed())
			Expect(runState.Set("task_tree", "pending", "false")).To(Succeed())

			ip := behtree.NewInterpreter(nil, result.Registry, runState)
			ip.SetOutcomeRequest(behtree.RequestSuccess)

			// First tick: reaches idle
			status, tickErr := ip.Tick(result.Tree)
			Expect(tickErr).NotTo(HaveOccurred())
			Expect(status).To(Equal(behtree.Success))

			// Simulate external event: speech starts
			Expect(runState.Set("speech", "active", "true")).To(Succeed())

			// Second tick: idle was reset, tree re-evaluates and handles speech
			status, tickErr = ip.Tick(result.Tree)
			Expect(tickErr).NotTo(HaveOccurred())
			Expect(status).To(Equal(behtree.Success))

			speech, _ := runState.Get("speech", "active")
			Expect(speech).To(Equal("false"))
		})
	})
})
