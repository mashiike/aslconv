comment  = "An example of the Amazon States Language using a map state."
start_at = state.map.Validate-All

state "map" "Validate-All" {
  max_concurrency = 0
  items_path      = "$.shipped"
  input_path      = "$.detail"
  result_path     = "$.detail.shipped"
  end             = true

  iterator {
    start_at = state.task.Validate

    state "task" "Validate" {
      resource        = "arn:aws:lambda:us-east-1:123456789012:function:ship-val"
      next            = state.wait.Wait
      output_path     = "$.items"
      retry           = ["{\"ErrorEquals\":[\"ErrorA\",\"ErrorB\"],\"IntervalSeconds\":1,\"BackoffRate\":2,\"MaxAttempts\":2}", "{\"ErrorEquals\":[\"ErrorC\"],\"IntervalSeconds\":5}"]
      catch           = ["{\"ErrorEquals\":[\"States.ALL\"],\"Next\":\"Z\"}"]
      parameters      = "{\"input.$\":\"$\"}"
      result_selector = "{\"data.$\":\"$\"}"
    }

    state "wait" "Wait" {
      seconds = 10
      next    = state.pass.Pass
    }

    state "pass" "Pass" {
      next        = state.succeed.Success
      result_path = "$.coords"
      result      = "{\"x-datum\":0.381018,\"y-datum\":622.2269926397355}"
    }

    state "succeed" "Success" {
    }
  }
}
