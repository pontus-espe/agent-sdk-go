package model_test

// Manual smoke test for streaming against a live provider. It is kept commented
// out because it needs real credentials; see test/model for the automated tests.
//
// func TestRunStreaming(t *testing.T) {
// 	ctx := context.Background()
// 	r, assistant, _ := setupMultiAgent("azure")
// 	result, err := r.RunStreaming(ctx, assistant, &runner.RunOptions{
// 		Input: `Please tell me today's weather in Japan.(use get_todays_date from tools.)`,
// 		RunConfig: &runner.RunConfig{
// 			TracingDisabled: true,
// 		},
// 	})
// 	if err != nil {
// 		log.Fatalf("Error running agent: %v", err)
// 	}
//
// 	for st := range result.Stream {
// 		switch st.Type {
// 		case model.StreamEventTypeError:
// 			log.Fatalf("Error stream: %v", st.Error)
// 		}
// 		if result.IsComplete {
// 			break
// 		}
// 	}
// 	fmt.Println(result.FinalOutput)
// }
