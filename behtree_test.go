package behtree_test

import (
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/richiejp/behtree"
	"github.com/richiejp/behtree/internal/benchcases"
)

var _ = Describe("Document Parsing", func() {
	Context("robot example", func() {
		var doc *behtree.Document
		var err error

		BeforeEach(func() {
			data, readErr := os.ReadFile("testdata/robot.json")
			Expect(readErr).NotTo(HaveOccurred())
			doc, err = behtree.ParseDocument(data)
		})

		It("parses without error", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(doc).NotTo(BeNil())
		})

		It("has objects", func() {
			Expect(doc.Objects).To(HaveLen(4))
			Expect(doc.Objects[0].Name).To(Equal("wrapper"))
		})

		It("has behaviours", func() {
			Expect(doc.Behaviours).To(HaveLen(5))
		})

		It("has a tree", func() {
			Expect(doc.Tree).NotTo(BeNil())
			Expect(doc.Tree.Type).To(Equal(behtree.SequenceNode))
		})
	})

	Context("desktop example with multiple files", func() {
		var env *behtree.Environment

		BeforeEach(func() {
			var err error
			env, err = behtree.LoadEnvironment(
				"testdata/desktop_env.json",
				"testdata/desktop_outer.json",
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("merges objects from env file", func() {
			Expect(env.Objects).To(HaveLen(4))
		})

		It("merges interfaces from env file", func() {
			Expect(env.Interfaces).To(HaveLen(3))
		})

		It("merges behaviours from env file", func() {
			Expect(env.Behaviours).To(HaveLen(13))
		})

		It("has the outer tree", func() {
			Expect(env.Trees).To(HaveLen(1))
			Expect(env.Trees[0].Type).To(Equal(behtree.FallbackNode))
		})
	})
})

var _ = Describe("Validation", func() {
	Context("valid robot tree", func() {
		It("produces no errors", func() {
			env, err := behtree.LoadEnvironment("testdata/robot.json")
			Expect(err).NotTo(HaveOccurred())
			errs := behtree.Validate(env)
			Expect(errs).To(BeEmpty())
		})
	})

	Context("valid desktop environment + outer tree", func() {
		It("produces no errors", func() {
			env, err := behtree.LoadEnvironment(
				"testdata/desktop_env.json",
				"testdata/desktop_outer.json",
			)
			Expect(err).NotTo(HaveOccurred())
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
			Expect(errs[0].Message).To(ContainSubstring("unknown behaviour"))
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

		It("catches wrong node type for behaviour", func() {
			doc, err := behtree.ParseDocument([]byte(`{
				"behaviours": [{"name":"Foo","type":"Condition"}],
				"tree": {"type":"Action","name":"Foo"}
			}`))
			Expect(err).NotTo(HaveOccurred())
			env := behtree.MergeDocuments(doc)
			errs := behtree.Validate(env)
			Expect(errs).NotTo(BeEmpty())
			Expect(errs[0].Message).To(ContainSubstring("but used as"))
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
	It("prints the robot tree matching README format", func() {
		env, err := behtree.LoadEnvironment("testdata/robot.json")
		Expect(err).NotTo(HaveOccurred())
		tree := env.Trees[0]
		output := behtree.PrintTree(tree)

		Expect(output).To(ContainSubstring("Sequence"))
		Expect(output).To(ContainSubstring("├── Fallback"))
		Expect(output).To(ContainSubstring("Condition: IsHolding"))
		Expect(output).To(ContainSubstring("Action: NavigateTo"))
		Expect(output).To(ContainSubstring("Action: DropIn"))
		Expect(output).To(ContainSubstring("└── Action: DropIn"))

		lines := strings.Split(strings.TrimSpace(output), "\n")
		Expect(lines[0]).To(Equal("Sequence"))
	})
})

var _ = Describe("Interpreter", func() {
	var (
		env      *behtree.Environment
		registry *behtree.BehaviourRegistry
		state    *behtree.State
	)

	Context("robot example tick-by-tick", func() {
		BeforeEach(func() {
			var err error
			env, err = behtree.LoadEnvironment("testdata/robot.json")
			Expect(err).NotTo(HaveOccurred())

			registry = behtree.NewBehaviourRegistry()
			state = behtree.NewStateFromEnvironment(env)

			Expect(state.Set("robot", "location", "start")).To(Succeed())
			Expect(state.Set("robot", "holding", "")).To(Succeed())
			Expect(state.Set("wrapper", "location", "table")).To(Succeed())

			benchcases.RegisterRobotHandlers(registry)
		})

		It("completes the task over multiple ticks", func() {
			tree := env.Trees[0]
			ip := behtree.NewInterpreter(env, registry, state)

			ip.SetOutcomeRequest(behtree.RequestRunning)
			status, err := ip.Tick(tree)
			Expect(err).NotTo(HaveOccurred())
			Expect(status).To(Equal(behtree.Running))

			robotLoc, _ := state.Get("robot", "location")
			Expect(robotLoc).To(Equal("start"))

			ip.SetOutcomeRequest(behtree.RequestSuccess)
			status, err = ip.Tick(tree)
			Expect(err).NotTo(HaveOccurred())
			Expect(status).To(Equal(behtree.Success))

			loc, _ := state.Get("robot", "location")
			Expect(loc).To(Equal("bin"))

			holding, _ := state.Get("robot", "holding")
			Expect(holding).To(Equal(""))

			wrapperLoc, _ := state.Get("wrapper", "location")
			Expect(wrapperLoc).To(Equal("bin"))
		})

		It("reaches success when all actions succeed immediately", func() {
			tree := env.Trees[0]
			ip := behtree.NewInterpreter(env, registry, state)
			ip.SetOutcomeRequest(behtree.RequestSuccess)

			var finalStatus behtree.Status
			for i := 0; i < 10; i++ {
				status, err := ip.Tick(tree)
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
	It("executes a subtree from state", func() {
		env, err := behtree.LoadEnvironment(
			"testdata/desktop_env.json",
			"testdata/desktop_outer.json",
		)
		Expect(err).NotTo(HaveOccurred())

		innerDoc, err := behtree.LoadDocument("testdata/desktop_inner.json")
		Expect(err).NotTo(HaveOccurred())

		registry := behtree.NewBehaviourRegistry()
		state := behtree.NewStateFromEnvironment(env)

		Expect(state.Set("task_tree", "tree", innerDoc.Tree)).To(Succeed())

		registry.Register("HasTaskTree", func(params behtree.Params, s *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
			val, _ := s.Get("task_tree", "tree")
			if val != nil {
				return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
			}
			return behtree.HandlerResult{Status: behtree.Failure, Compatible: true}
		})

		registry.Register("HasPendingUtterance", func(params behtree.Params, s *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
			return behtree.HandlerResult{Status: behtree.Failure, Compatible: true}
		})

		registry.Register("UserSpeaking", func(params behtree.Params, s *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
			return behtree.HandlerResult{Status: behtree.Failure, Compatible: true}
		})

		registry.Register("WaitForSpeech", func(params behtree.Params, s *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
			return behtree.HandlerResult{Status: behtree.Running, Compatible: true}
		})

		registry.Register("ProcessUtterance", func(params behtree.Params, s *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
			return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
		})

		registry.Register("QuerySwayTree", func(params behtree.Params, s *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
			return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
		})

		registry.Register("IsAppOpen", func(params behtree.Params, s *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
			return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
		})

		registry.Register("IsPageOpen", func(params behtree.Params, s *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
			return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
		})

		registry.Register("IsFocused", func(params behtree.Params, s *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
			return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
		})

		registry.Register("FocusWindow", func(params behtree.Params, s *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
			return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
		})

		registry.Register("OpenApp", func(params behtree.Params, s *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
			return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
		})

		registry.Register("OpenURL", func(params behtree.Params, s *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
			return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
		})

		tree := env.Trees[0]
		ip := behtree.NewInterpreter(env, registry, state)
		ip.SetOutcomeRequest(behtree.RequestSuccess)

		status, err := ip.Tick(tree)
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal(behtree.Success))
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
