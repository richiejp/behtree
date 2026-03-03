package behtree_test

import (
	"bytes"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/richiejp/behtree"
)

var _ = Describe("Tracing", func() {
	var (
		env      *behtree.Environment
		registry *behtree.BehaviourRegistry
		tree     *behtree.Node
	)

	BeforeEach(func() {
		tree = &behtree.Node{
			Type: behtree.SequenceNode,
			Children: []*behtree.Node{
				{
					Type: behtree.ConditionNode,
					Name: "IsReady",
				},
				{
					Type:   behtree.ActionNode,
					Name:   "DoWork",
					Params: behtree.Params{"target": "foo"},
				},
			},
		}

		env = &behtree.Environment{
			Objects: []behtree.ObjectDef{
				{Name: "worker", Fields: map[string]behtree.FieldType{"status": behtree.FieldString}},
			},
			Behaviours: []behtree.BehaviourDef{
				{Name: "IsReady", Type: behtree.ConditionNode},
				{Name: "DoWork", Type: behtree.ActionNode, Params: map[string]behtree.ParamType{"target": behtree.ParamString}},
			},
			Trees: []*behtree.Node{tree},
		}

		registry = behtree.NewBehaviourRegistry()
	})

	Context("RecordingTracer", func() {
		It("builds correct span tree structure", func() {
			registry.Register("IsReady", func(_ behtree.Params, _ *behtree.State, _ behtree.OutcomeRequest) behtree.HandlerResult {
				return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
			})
			registry.Register("DoWork", func(_ behtree.Params, s *behtree.State, _ behtree.OutcomeRequest) behtree.HandlerResult {
				_ = s.Set("worker", "status", "done")
				return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
			})

			state := behtree.NewStateFromEnvironment(env)
			ip := behtree.NewInterpreter(env, registry, state)
			recorder := behtree.NewRecordingTracer(true)
			ip.SetTracer(recorder)

			status, err := ip.Tick(tree)
			Expect(err).NotTo(HaveOccurred())
			Expect(status).To(Equal(behtree.Success))

			root := recorder.Root()
			Expect(root).NotTo(BeNil())
			Expect(root.NodeType).To(Equal(behtree.SequenceNode))
			Expect(root.Status).To(Equal(behtree.Success))
			Expect(root.Children).To(HaveLen(2))

			// First child: Condition
			Expect(root.Children[0].NodeType).To(Equal(behtree.ConditionNode))
			Expect(root.Children[0].NodeName).To(Equal("IsReady"))
			Expect(root.Children[0].Status).To(Equal(behtree.Success))

			// Second child: Action with state captured
			Expect(root.Children[1].NodeType).To(Equal(behtree.ActionNode))
			Expect(root.Children[1].NodeName).To(Equal("DoWork"))
			Expect(root.Children[1].Status).To(Equal(behtree.Success))
			Expect(root.Children[1].StateAfter).NotTo(BeNil())
			Expect(root.Children[1].StateAfter["worker"]["status"]).To(Equal("done"))
		})

		It("captures handler logs", func() {
			registry.Register("IsReady", func(_ behtree.Params, _ *behtree.State, _ behtree.OutcomeRequest) behtree.HandlerResult {
				return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
			})
			registry.Register("DoWork", func(_ behtree.Params, _ *behtree.State, _ behtree.OutcomeRequest) behtree.HandlerResult {
				return behtree.HandlerResult{
					Status:     behtree.Success,
					Compatible: true,
					Logs: []behtree.LogEntry{
						{Level: behtree.LogInfo, Message: "starting work on target"},
						{Level: behtree.LogError, Message: "target not found"},
					},
				}
			})

			state := behtree.NewStateFromEnvironment(env)
			ip := behtree.NewInterpreter(env, registry, state)
			recorder := behtree.NewRecordingTracer(false)
			ip.SetTracer(recorder)

			_, _ = ip.Tick(tree)

			root := recorder.Root()
			Expect(root.Children[1].Logs).To(HaveLen(2))
			Expect(root.Children[1].Logs[0].Level).To(Equal(behtree.LogInfo))
			Expect(root.Children[1].Logs[0].Message).To(Equal("starting work on target"))
			Expect(root.Children[1].Logs[1].Level).To(Equal(behtree.LogError))
		})

		It("records failure with short-circuit", func() {
			registry.Register("IsReady", func(_ behtree.Params, _ *behtree.State, _ behtree.OutcomeRequest) behtree.HandlerResult {
				return behtree.HandlerResult{Status: behtree.Failure, Compatible: true}
			})
			registry.Register("DoWork", func(_ behtree.Params, _ *behtree.State, _ behtree.OutcomeRequest) behtree.HandlerResult {
				return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
			})

			state := behtree.NewStateFromEnvironment(env)
			ip := behtree.NewInterpreter(env, registry, state)
			recorder := behtree.NewRecordingTracer(false)
			ip.SetTracer(recorder)

			status, err := ip.Tick(tree)
			Expect(err).NotTo(HaveOccurred())
			Expect(status).To(Equal(behtree.Failure))

			root := recorder.Root()
			Expect(root.Status).To(Equal(behtree.Failure))
			// Sequence short-circuited: only the failing child was evaluated
			Expect(root.Children).To(HaveLen(1))
			Expect(root.Children[0].NodeName).To(Equal("IsReady"))
			Expect(root.Children[0].Status).To(Equal(behtree.Failure))
		})

		It("resets between ticks", func() {
			registry.Register("IsReady", func(_ behtree.Params, _ *behtree.State, _ behtree.OutcomeRequest) behtree.HandlerResult {
				return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
			})
			registry.Register("DoWork", func(_ behtree.Params, _ *behtree.State, _ behtree.OutcomeRequest) behtree.HandlerResult {
				return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
			})

			state := behtree.NewStateFromEnvironment(env)
			ip := behtree.NewInterpreter(env, registry, state)
			recorder := behtree.NewRecordingTracer(false)
			ip.SetTracer(recorder)

			_, _ = ip.Tick(tree)
			Expect(recorder.Root().Children).To(HaveLen(2))

			recorder.Reset()
			Expect(recorder.Root()).To(BeNil())

			_, _ = ip.Tick(tree)
			Expect(recorder.Root().Children).To(HaveLen(2))
		})
	})

	Context("Harness with tracing", func() {
		It("populates Trace on ScenarioResult when enabled", func() {
			registry.Register("IsReady", func(_ behtree.Params, _ *behtree.State, _ behtree.OutcomeRequest) behtree.HandlerResult {
				return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
			})
			registry.Register("DoWork", func(_ behtree.Params, _ *behtree.State, _ behtree.OutcomeRequest) behtree.HandlerResult {
				return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
			})

			harness := behtree.NewSimulationHarness(env, registry, tree)
			harness.SetTracing(true)
			harness.SetCaptureState(true)

			state := behtree.NewStateFromEnvironment(env)
			result := harness.RunScenario(
				[]behtree.OutcomeRequest{behtree.RequestSuccess, behtree.RequestSuccess},
				state, 10,
			)

			Expect(result.Trace).NotTo(BeNil())
			Expect(result.Trace.Ticks).To(HaveLen(1))
			Expect(result.Trace.Ticks[0].Root).NotTo(BeNil())
			Expect(result.Trace.Ticks[0].Root.NodeType).To(Equal(behtree.SequenceNode))
		})

		It("leaves Trace nil when tracing is disabled", func() {
			registry.Register("IsReady", func(_ behtree.Params, _ *behtree.State, _ behtree.OutcomeRequest) behtree.HandlerResult {
				return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
			})
			registry.Register("DoWork", func(_ behtree.Params, _ *behtree.State, _ behtree.OutcomeRequest) behtree.HandlerResult {
				return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
			})

			harness := behtree.NewSimulationHarness(env, registry, tree)

			state := behtree.NewStateFromEnvironment(env)
			result := harness.RunScenario(
				[]behtree.OutcomeRequest{behtree.RequestSuccess, behtree.RequestSuccess},
				state, 10,
			)

			Expect(result.Trace).To(BeNil())
		})
	})

	Context("Trace I/O round-trip", func() {
		It("writes and reads back traces correctly", func() {
			reqSuccess := behtree.RequestSuccess
			reqFailure := behtree.RequestFailure
			traces := []*behtree.ScenarioTrace{
				{
					Requests: []behtree.OutcomeRequest{behtree.RequestSuccess},
					Ticks: []behtree.TickTrace{
						{
							TickIndex: 0,
							Root: &behtree.Span{
								NodeType: behtree.SequenceNode,
								Status:   behtree.Success,
								Children: []*behtree.Span{
									{
										NodeType:       behtree.ConditionNode,
										NodeName:       "Check",
										OutcomeRequest: &reqSuccess,
										Status:         behtree.Success,
										Logs: []behtree.LogEntry{
											{Level: behtree.LogInfo, Message: "all good"},
										},
									},
								},
							},
						},
					},
					Failed: false,
				},
				{
					Requests: []behtree.OutcomeRequest{behtree.RequestFailure},
					Ticks: []behtree.TickTrace{
						{
							TickIndex: 0,
							Root: &behtree.Span{
								NodeType: behtree.SequenceNode,
								Status:   behtree.Failure,
								Children: []*behtree.Span{
									{
										NodeType:       behtree.ConditionNode,
										NodeName:       "Check",
										OutcomeRequest: &reqFailure,
										Status:         behtree.Failure,
									},
								},
							},
						},
					},
					Failed: true,
				},
			}

			meta := behtree.TraceMetadata{
				CaseName:  "TestCase",
				Model:     "test-model",
				Timestamp: "2025-01-01T00:00:00Z",
				TreeJSON:  `{"type":"Sequence"}`,
			}

			var buf bytes.Buffer
			err := behtree.WriteTraces(&buf, meta, traces)
			Expect(err).NotTo(HaveOccurred())

			// Read metadata
			reader := bytes.NewReader(buf.Bytes())
			readMeta, err := behtree.ReadTraceMetadata(reader)
			Expect(err).NotTo(HaveOccurred())
			Expect(readMeta.CaseName).To(Equal("TestCase"))
			Expect(readMeta.Model).To(Equal("test-model"))

			// Read traces
			reader = bytes.NewReader(buf.Bytes())
			var readTraces []*behtree.ScenarioTrace
			err = behtree.ReadScenarioTraces(reader, func(_ int, trace *behtree.ScenarioTrace) bool {
				readTraces = append(readTraces, trace)
				return true
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(readTraces).To(HaveLen(2))
			Expect(readTraces[0].Failed).To(BeFalse())
			Expect(readTraces[1].Failed).To(BeTrue())

			// Verify span structure survived round-trip
			Expect(readTraces[0].Ticks[0].Root.Children).To(HaveLen(1))
			Expect(readTraces[0].Ticks[0].Root.Children[0].Logs).To(HaveLen(1))
			Expect(readTraces[0].Ticks[0].Root.Children[0].Logs[0].Message).To(Equal("all good"))
		})

		It("stops reading when callback returns false", func() {
			traces := make([]*behtree.ScenarioTrace, 5)
			for i := range traces {
				traces[i] = &behtree.ScenarioTrace{
					Requests: []behtree.OutcomeRequest{behtree.OutcomeRequest(i % 3)},
					Failed:   i%2 == 0,
				}
			}

			meta := behtree.TraceMetadata{CaseName: "Stop"}

			var buf bytes.Buffer
			Expect(behtree.WriteTraces(&buf, meta, traces)).To(Succeed())

			reader := bytes.NewReader(buf.Bytes())
			count := 0
			_ = behtree.ReadScenarioTraces(reader, func(_ int, _ *behtree.ScenarioTrace) bool {
				count++
				return count < 2
			})
			Expect(count).To(Equal(2))
		})
	})

	Context("Trace printing", func() {
		It("renders span tree as ASCII", func() {
			span := &behtree.Span{
				NodeType: behtree.SequenceNode,
				Status:   behtree.Failure,
				Children: []*behtree.Span{
					{
						NodeType: behtree.ConditionNode,
						NodeName: "IsReady",
						Status:   behtree.Success,
					},
					{
						NodeType: behtree.ActionNode,
						NodeName: "DoWork",
						Params:   behtree.Params{"target": "foo"},
						Status:   behtree.Failure,
						Err:      "something went wrong",
						Logs: []behtree.LogEntry{
							{Level: behtree.LogError, Message: "failed to process"},
						},
					},
				},
			}

			var buf bytes.Buffer
			behtree.PrintSpanTree(span, &buf)
			output := buf.String()

			Expect(output).To(ContainSubstring("Sequence -> FAILURE"))
			Expect(output).To(ContainSubstring("Condition: IsReady -> SUCCESS"))
			Expect(output).To(ContainSubstring("Action: DoWork"))
			Expect(output).To(ContainSubstring("FAILURE"))
			Expect(output).To(ContainSubstring("[error] failed to process"))
		})

		It("renders full scenario trace", func() {
			trace := &behtree.ScenarioTrace{
				Requests: []behtree.OutcomeRequest{behtree.RequestSuccess},
				Ticks: []behtree.TickTrace{
					{
						TickIndex: 0,
						Root: &behtree.Span{
							NodeType: behtree.SequenceNode,
							Status:   behtree.Success,
						},
					},
				},
				FinalState: &behtree.State{
					Objects: map[string]map[string]any{
						"worker": {"status": "done"},
					},
				},
			}

			var buf bytes.Buffer
			behtree.PrintScenarioTrace(trace, &buf)
			output := buf.String()

			_, _ = fmt.Fprintln(GinkgoWriter, output)

			Expect(output).To(ContainSubstring("Requests: [RequestSuccess]"))
			Expect(output).To(ContainSubstring("Tick 1"))
			Expect(output).To(ContainSubstring("Final State:"))
			Expect(output).To(ContainSubstring("worker.status = done"))
		})
	})
})
