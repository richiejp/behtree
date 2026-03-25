package behtree_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/richiejp/behtree"
	"github.com/richiejp/behtree/internal/benchcases"
)

var _ = Describe("Document Parsing", func() {
	Context("robot v2 example", func() {
		var doc *behtree.Document
		var err error

		BeforeEach(func() {
			doc, err = behtree.LoadDocument("testdata/robot_v2.json")
		})

		It("parses without error", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(doc).NotTo(BeNil())
		})

		It("has objects", func() {
			Expect(doc.Objects).To(HaveLen(4))
			Expect(doc.Objects[0].Name).To(Equal("wrapper"))
		})

		It("has actions with preconditions", func() {
			Expect(doc.Actions).To(HaveLen(4))
			// PickUp has preconditions (location, holding, observed, robot.observed)
			Expect(doc.Actions[2].Name).To(Equal("PickUp"))
			Expect(doc.Actions[2].Preconditions).To(HaveLen(4))
		})

		It("has a goal", func() {
			Expect(doc.Goal).To(HaveLen(1))
			Expect(doc.Goal[0].Object).To(Equal("wrapper"))
			Expect(doc.Goal[0].Field).To(Equal("location"))
			Expect(doc.Goal[0].Value).To(Equal("bin"))
		})
	})

	Context("desktop v2 example with multiple files", func() {
		var env *behtree.Environment

		BeforeEach(func() {
			var err error
			env, err = behtree.LoadEnvironment(
				"testdata/desktop_v2.json",
				"testdata/desktop_v2_outer.json",
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("merges objects from both environments", func() {
			// inner: sway_state, firefox, utterance, task_tree
			// outer: speech, task_tree, system (task_tree duplicated)
			Expect(env.Objects).To(HaveLen(7))
		})

		It("merges actions from both environments", func() {
			// inner: Observe, QuerySwayTree, OpenApp, OpenURL, WaitForSpeech, ProcessUtterance, RunTaskTree
			// outer: Observe, HandleSpeech, RunTaskTree, Idle (Observe + RunTaskTree duplicated)
			Expect(env.Actions).To(HaveLen(11))
		})

		It("has the outer goal (last loaded wins)", func() {
			Expect(env.Goal).To(HaveLen(2))
			Expect(env.Goal[0].Object).To(Equal("speech"))
			Expect(env.Goal[0].Field).To(Equal("observed"))
			Expect(env.Goal[1].Object).To(Equal("system"))
			Expect(env.Goal[1].Field).To(Equal("idle"))
		})

		It("has no pre-built trees (both use PA-BT)", func() {
			Expect(env.Trees).To(BeEmpty())
		})
	})
})

var _ = Describe("Validation", func() {
	Context("action-only environment", func() {
		It("produces no errors for action behaviours", func() {
			doc, err := behtree.ParseDocument([]byte(`{
				"objects": [{"name":"robot","fields":{"location":"string"}}],
				"behaviours": [
					{"name":"NavigateTo","type":"Action","params":{"location":"object_ref"}}
				],
				"tree": {
					"type":"Sequence",
					"children":[
						{"type":"Condition","name":"IsAtTarget"},
						{"type":"Action","name":"NavigateTo","params":{"location":"robot"}}
					]
				}
			}`))
			Expect(err).NotTo(HaveOccurred())
			env := behtree.MergeDocuments(doc)
			errs := behtree.Validate(env)
			Expect(errs).To(BeEmpty())
		})
	})

	Context("invalid tree", func() {
		It("catches unknown behaviour", func() {
			doc, err := behtree.ParseDocument([]byte(`{
				"behaviours": [{"name":"Foo","type":"Action"}],
				"tree": {"type":"Action","name":"Bar"}
			}`))
			Expect(err).NotTo(HaveOccurred())
			env := behtree.MergeDocuments(doc)
			errs := behtree.Validate(env)
			Expect(errs).NotTo(BeEmpty())
			Expect(errs[0].Message).To(ContainSubstring("unknown action"))
		})

		It("catches missing children on Sequence", func() {
			doc, err := behtree.ParseDocument([]byte(`{
				"tree": {"type":"Sequence"}
			}`))
			Expect(err).NotTo(HaveOccurred())
			env := behtree.MergeDocuments(doc)
			errs := behtree.Validate(env)
			Expect(errs).NotTo(BeEmpty())
			Expect(errs[0].Message).To(ContainSubstring("at least one child"))
		})

		It("rejects condition behaviour definitions", func() {
			doc, err := behtree.ParseDocument([]byte(`{
				"behaviours": [{"name":"Foo","type":"Condition"}],
				"tree": {"type":"Condition","name":"Foo"}
			}`))
			Expect(err).NotTo(HaveOccurred())
			env := behtree.MergeDocuments(doc)
			errs := behtree.Validate(env)
			Expect(errs).NotTo(BeEmpty())
			Expect(errs[0].Message).To(ContainSubstring("behaviour type must be Action"))
		})

		It("catches unknown object reference", func() {
			doc, err := behtree.ParseDocument([]byte(`{
				"behaviours": [{"name":"Foo","type":"Action","params":{"obj":"object_ref"}}],
				"tree": {"type":"Action","name":"Foo","params":{"obj":"nonexistent"}}
			}`))
			Expect(err).NotTo(HaveOccurred())
			env := behtree.MergeDocuments(doc)
			errs := behtree.Validate(env)
			Expect(errs).NotTo(BeEmpty())
			Expect(errs[0].Message).To(ContainSubstring("unknown object"))
		})

		It("catches duplicate objects", func() {
			doc, err := behtree.ParseDocument([]byte(`{
				"objects": [{"name":"a","fields":{}},{"name":"a","fields":{}}]
			}`))
			Expect(err).NotTo(HaveOccurred())
			env := behtree.MergeDocuments(doc)
			errs := behtree.Validate(env)
			Expect(errs).NotTo(BeEmpty())
			Expect(errs[0].Message).To(ContainSubstring("duplicate object"))
		})
	})
})

var _ = Describe("Pretty Print", func() {
	It("prints a PA-BT generated tree", func() {
		state := robotTestState()
		result, err := behtree.BuildTree(robotTestGoal(), robotTestActions(), state)
		Expect(err).NotTo(HaveOccurred())

		output := behtree.PrintTree(result.Tree)
		Expect(output).To(ContainSubstring("Sequence"))
		Expect(output).To(ContainSubstring("Fallback"))
		Expect(output).To(ContainSubstring("Action: NavigateTo"))
		Expect(output).To(ContainSubstring("Action: DropIn"))

		lines := strings.Split(strings.TrimSpace(output), "\n")
		Expect(lines[0]).To(Equal("Sequence"))
	})
})

var _ = Describe("Interpreter", func() {
	Context("PA-BT robot example tick-by-tick", func() {
		var (
			result *behtree.BuildResult
			state  *behtree.State
		)

		BeforeEach(func() {
			state = robotTestState()
			var err error
			result, err = behtree.BuildTree(robotTestGoal(), robotTestActions(), state)
			Expect(err).NotTo(HaveOccurred())
			benchcases.RegisterRobotHandlers(result.Registry)
		})

		It("reaches success when all actions succeed immediately", func() {
			ip := behtree.NewInterpreter(nil, result.Registry, state)
			ip.SetOutcomeRequest(behtree.RequestSuccess)

			var finalStatus behtree.Status
			for range 20 {
				status, err := ip.Tick(result.Tree)
				Expect(err).NotTo(HaveOccurred())
				finalStatus = status
				if status == behtree.Success {
					break
				}
			}
			Expect(finalStatus).To(Equal(behtree.Success))

			wrapperLoc, _ := state.Get("wrapper", "location")
			Expect(wrapperLoc).To(Equal("bin"))
		})
	})
})

var _ = Describe("RunTaskTree", func() {
	It("executes a PA-BT inner tree via PA-BT outer tree", func() {
		// Build inner tree via PA-BT
		innerState := desktopTestState()
		innerResult, err := behtree.BuildTree(desktopTestGoal(), desktopTestActions(), innerState)
		Expect(err).NotTo(HaveOccurred())

		// Build outer tree via PA-BT
		outerState := desktopOuterTestState()
		outerResult, err := behtree.BuildTree(desktopOuterTestGoal(), desktopOuterTestActions(), outerState)
		Expect(err).NotTo(HaveOccurred())

		// Merge registries: outer PA-BT conditions + inner PA-BT conditions
		registry := outerResult.Registry
		registry.Merge(innerResult.Registry)

		// Register action handlers
		benchcases.RegisterDesktopInnerHandlers(registry)
		benchcases.RegisterDesktopOuterHandlers(registry)

		// Set up combined runtime state
		state := behtree.NewState()
		state.EphemeralFields = []string{"observed", "idle"}
		state.Objects["speech"] = map[string]any{"active": "true", "observed": "false"}
		state.Objects["task_tree"] = map[string]any{"pending": "true", "tree": innerResult.Tree}
		state.Objects["system"] = map[string]any{"idle": "false"}
		state.Objects["sway_state"] = map[string]any{"refreshed": "false"}
		state.Objects["firefox"] = map[string]any{"open": "false", "active_page": "", "observed": "false"}

		ip := behtree.NewInterpreter(nil, registry, state)
		ip.SetOutcomeRequest(behtree.RequestSuccess)

		var finalStatus behtree.Status
		for range 20 {
			status, tickErr := ip.Tick(outerResult.Tree)
			Expect(tickErr).NotTo(HaveOccurred())
			finalStatus = status
			if status == behtree.Success {
				break
			}
		}
		Expect(finalStatus).To(Equal(behtree.Success))

		// Verify the inner tree executed and changed state
		activePage, _ := state.Get("firefox", "active_page")
		Expect(activePage).To(Equal(desktopTestURL))

		// Verify outer tree reached idle
		idle, _ := state.Get("system", "idle")
		Expect(idle).To(Equal("true"))
	})
})

var _ = Describe("SimulationHarness", func() {
	It("runs a simple scenario", func() {
		doc, err := behtree.ParseDocument([]byte(`{
			"behaviours": [
				{"name":"Check","type":"Condition"},
				{"name":"DoIt","type":"Action"}
			],
			"tree": {
				"type":"Sequence",
				"children":[
					{"type":"Condition","name":"Check"},
					{"type":"Action","name":"DoIt"}
				]
			}
		}`))
		Expect(err).NotTo(HaveOccurred())
		env := behtree.MergeDocuments(doc)

		registry := behtree.NewBehaviourRegistry()
		registry.Register("Check", func(params behtree.Params, s *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
			return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
		})
		registry.Register("DoIt", func(params behtree.Params, s *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
			return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
		})

		harness := behtree.NewSimulationHarness(env, registry, env.Trees[0])
		state := behtree.NewState()
		result := harness.RunScenario(
			[]behtree.OutcomeRequest{behtree.RequestSuccess, behtree.RequestSuccess},
			state,
			10,
		)
		Expect(result.Skipped).To(BeFalse())
		Expect(result.Ticks).To(HaveLen(1))
		Expect(result.Ticks[0].Status).To(Equal(behtree.Success))
	})

	Context("per-leaf request consumption", func() {
		// Regression tests for the bug where requests were consumed per-tick
		// instead of per-leaf-visit. The old code applied a single OutcomeRequest
		// to ALL leaves in a tick, making it impossible to request different
		// outcomes for different leaves within the same tick.

		var (
			env      *behtree.Environment
			registry *behtree.BehaviourRegistry
		)

		// Tree: Sequence[ Condition:Check, Action:DoIt ]
		// Two leaves visited per tick when Check succeeds.
		BeforeEach(func() {
			doc, err := behtree.ParseDocument([]byte(`{
				"behaviours": [
					{"name":"Check","type":"Condition"},
					{"name":"DoIt","type":"Action"}
				],
				"tree": {
					"type":"Sequence",
					"children":[
						{"type":"Condition","name":"Check"},
						{"type":"Action","name":"DoIt"}
					]
				}
			}`))
			Expect(err).NotTo(HaveOccurred())
			env = behtree.MergeDocuments(doc)

			registry = behtree.NewBehaviourRegistry()
			// Check: obeys the requested outcome
			registry.Register("Check", func(_ behtree.Params, _ *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
				switch req {
				case behtree.RequestFailure:
					return behtree.HandlerResult{Status: behtree.Failure, Compatible: true}
				default:
					return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
				}
			})
			// DoIt: obeys the requested outcome
			registry.Register("DoIt", func(_ behtree.Params, _ *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
				switch req {
				case behtree.RequestFailure:
					return behtree.HandlerResult{Status: behtree.Failure, Compatible: true}
				case behtree.RequestRunning:
					return behtree.HandlerResult{Status: behtree.Running, Compatible: true}
				default:
					return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
				}
			})
		})

		It("gives different requests to different leaves in the same tick", func() {
			harness := behtree.NewSimulationHarness(env, registry, env.Trees[0])
			harness.SetTracing(true)
			state := behtree.NewState()

			// Request[0]=Success for Check, Request[1]=Running for DoIt
			result := harness.RunScenario(
				[]behtree.OutcomeRequest{behtree.RequestSuccess, behtree.RequestRunning},
				state, 10,
			)
			Expect(result.Skipped).To(BeFalse())
			// Tick 1: Check=Success, DoIt=Running → tree returns RUNNING
			Expect(result.Ticks[0].Status).To(Equal(behtree.Running))

			// Verify per-leaf requests in the trace
			root := result.Trace.Ticks[0].Root
			Expect(root.Children).To(HaveLen(2))
			Expect(*root.Children[0].OutcomeRequest).To(Equal(behtree.RequestSuccess))
			Expect(*root.Children[1].OutcomeRequest).To(Equal(behtree.RequestRunning))
		})

		It("consumes one request per leaf visit not per tick", func() {
			harness := behtree.NewSimulationHarness(env, registry, env.Trees[0])
			state := behtree.NewState()

			// 4 requests: tick 1 consumes [0] and [1], tick 2 consumes [2] and [3]
			result := harness.RunScenario(
				[]behtree.OutcomeRequest{
					behtree.RequestSuccess, behtree.RequestRunning, // tick 1: Check=S, DoIt=R → RUNNING
					behtree.RequestSuccess, behtree.RequestSuccess, // tick 2: Check=S, DoIt=S → SUCCESS
				},
				state, 10,
			)
			Expect(result.Skipped).To(BeFalse())
			Expect(result.Ticks).To(HaveLen(2))
			Expect(result.Ticks[0].Status).To(Equal(behtree.Running))
			Expect(result.Ticks[1].Status).To(Equal(behtree.Success))
		})

		It("allows condition to fail while action would succeed", func() {
			harness := behtree.NewSimulationHarness(env, registry, env.Trees[0])
			state := behtree.NewState()

			// Request[0]=Failure for Check → Sequence short-circuits, DoIt never reached
			// Only 1 request consumed in this tick.
			result := harness.RunScenario(
				[]behtree.OutcomeRequest{behtree.RequestFailure, behtree.RequestSuccess},
				state, 10,
			)
			Expect(result.Skipped).To(BeFalse())
			Expect(result.Ticks).To(HaveLen(1))
			Expect(result.Ticks[0].Status).To(Equal(behtree.Failure))
		})

		It("defaults to RequestSuccess when requests are exhausted", func() {
			harness := behtree.NewSimulationHarness(env, registry, env.Trees[0])
			state := behtree.NewState()

			// Only 2 requests (enough for tick 1). Tick 2 defaults to RequestSuccess.
			result := harness.RunScenario(
				[]behtree.OutcomeRequest{
					behtree.RequestSuccess, behtree.RequestRunning, // tick 1: RUNNING
				},
				state, 10,
			)
			Expect(result.Skipped).To(BeFalse())
			Expect(result.Ticks).To(HaveLen(2))
			Expect(result.Ticks[0].Status).To(Equal(behtree.Running))
			Expect(result.Ticks[1].Status).To(Equal(behtree.Success)) // defaults to Success
		})
	})

	Context("per-leaf requests with Fallback tree", func() {
		// Tree: Fallback[ Condition:IsReady, Action:Prepare ]
		// This is the pattern from the original bug report: a Fallback where
		// the condition checks state and the action can return different outcomes.

		It("handles different requests for condition and action in fallback", func() {
			doc, err := behtree.ParseDocument([]byte(`{
				"behaviours": [
					{"name":"IsReady","type":"Condition"},
					{"name":"Prepare","type":"Action"}
				],
				"tree": {
					"type":"Sequence",
					"children":[{
						"type":"Fallback",
						"children":[
							{"type":"Condition","name":"IsReady"},
							{"type":"Action","name":"Prepare"}
						]
					}]
				}
			}`))
			Expect(err).NotTo(HaveOccurred())
			env := behtree.MergeDocuments(doc)

			registry := behtree.NewBehaviourRegistry()
			registry.Register("IsReady", func(_ behtree.Params, _ *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
				if req == behtree.RequestSuccess {
					return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
				}
				return behtree.HandlerResult{Status: behtree.Failure, Compatible: true}
			})
			registry.Register("Prepare", func(_ behtree.Params, _ *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
				switch req {
				case behtree.RequestFailure:
					return behtree.HandlerResult{Status: behtree.Failure, Compatible: true}
				case behtree.RequestRunning:
					return behtree.HandlerResult{Status: behtree.Running, Compatible: true}
				default:
					return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
				}
			})

			harness := behtree.NewSimulationHarness(env, registry, env.Trees[0])
			harness.SetTracing(true)
			state := behtree.NewState()

			// Request[0]=Failure for IsReady, Request[1]=Running for Prepare
			// Fallback: IsReady fails → try Prepare → Running
			result := harness.RunScenario(
				[]behtree.OutcomeRequest{
					behtree.RequestFailure, behtree.RequestRunning, // tick 1
					behtree.RequestFailure, behtree.RequestSuccess, // tick 2
				},
				state, 10,
			)
			Expect(result.Skipped).To(BeFalse())
			Expect(result.Ticks).To(HaveLen(2))
			Expect(result.Ticks[0].Status).To(Equal(behtree.Running))
			Expect(result.Ticks[1].Status).To(Equal(behtree.Success))

			// Verify trace shows per-leaf requests
			tick1 := result.Trace.Ticks[0].Root
			fallback := tick1.Children[0]
			Expect(fallback.Children).To(HaveLen(2))
			Expect(*fallback.Children[0].OutcomeRequest).To(Equal(behtree.RequestFailure))
			Expect(*fallback.Children[1].OutcomeRequest).To(Equal(behtree.RequestRunning))
		})
	})
})
