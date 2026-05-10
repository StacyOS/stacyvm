import express from "express";
import { z } from "zod";
import { Client, ProviderError } from "stacyvm";

const requestSchema = z.object({
  code: z.string().min(1).max(50_000),
  timeout: z.string().default("10s"),
});

const app = express();
app.use(express.json({ limit: "64kb" }));

function stacyClient(): Client {
  return new Client({
    baseUrl: process.env.STACYVM_URL ?? "http://localhost:7423",
    apiKey: process.env.STACYVM_API_KEY,
    userId: process.env.STACYVM_USER_ID ?? "example-code-runner-typescript",
    timeout: 60_000,
  });
}

app.post("/run-javascript", async (request, response) => {
  const parsed = requestSchema.safeParse(request.body);
  if (!parsed.success) {
    response.status(400).json({ error: parsed.error.flatten() });
    return;
  }

  const client = stacyClient();
  const image = process.env.STACYVM_IMAGE ?? "node:20";
  const sandbox = await client.spawn({
    image,
    ttl: "2m",
    memory_mb: 512,
    vcpus: 1,
    metadata: { example: "code-runner-typescript" },
  });

  try {
    await sandbox.writeFile("/app/main.js", parsed.data.code);
    const result = await sandbox.exec("node /app/main.js", {
      timeout: parsed.data.timeout,
    });

    response.json({
      exit_code: result.exit_code,
      stdout: result.stdout,
      stderr: result.stderr,
      duration: result.duration,
    });
  } catch (error) {
    if (error instanceof ProviderError) {
      response.status(502).json({ error: error.message });
      return;
    }

    const message = error instanceof Error ? error.message : "unknown error";
    response.status(500).json({ error: message });
  } finally {
    await sandbox.destroy().catch(() => undefined);
  }
});

const port = Number(process.env.PORT ?? 8081);
app.listen(port, () => {
  console.log(`StacyVM TypeScript code runner listening on :${port}`);
});
