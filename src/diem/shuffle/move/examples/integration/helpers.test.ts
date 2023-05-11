// Copyright (c) The Diem Core Contributors
// SPDX-License-Identifier: Apache-2.0

import {
  assert,
  assertEquals,
} from "https://deno.land/std@0.85.0/testing/asserts.ts";
import * as context from "../main/context.ts";
import * as devapi from "../main/devapi.ts";
import * as helpers from "../main/helpers.ts";

Deno.test("invokeScriptFunction", async () => {
  const scriptFunction =
    context.senderAddress + "::Message::set_message";
  let txn = await helpers.invokeScriptFunction(
    scriptFunction,
    [],
    ["invoked script function"],
  );
  txn = await devapi.waitForTransactionCompletion(txn.hash);
  assert(txn.success);

  assertEquals(txn.vm_status, "Executed successfully");
  assertEquals(txn.payload.function, scriptFunction);
  assertEquals(
    helpers.hexToAscii(txn.payload.arguments[0]),
    "\x00invoked script function",
  );
});
