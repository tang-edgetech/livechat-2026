"use client";

import { useEffect, useState } from "react";
import {
  Alert,
  Button,
  Card,
  Descriptions,
  Input,
  Result,
  Space,
  Spin,
  Steps,
  Tag,
  Typography,
  message,
} from "antd";
import { CheckCircleFilled, CloseCircleFilled, MessageOutlined } from "@ant-design/icons";
import { useRouter } from "next/navigation";

import { apiGet, apiPost, ApiError } from "@/lib/api";

type CheckResult = { name: string; pass: boolean; note: string };

// Checks that must pass before continuing — Redis is optional/recommended
// only (overview.md §5 step 1: "warn if missing, don't block").
const BLOCKING_CHECKS = new Set(["Node.js", "Go backend", "MySQL", "Uploads directory writable"]);

type DBConfig = { host: string; port: string; name: string; user: string; password: string };
type SiteConfig = {
  title: string;
  timezone: string;
  baseUrl: string;
  uploadsPath: string;
  appPort: string;
  wsPort: string;
};
type AdminConfig = { fullName: string; email: string; password: string; confirmPassword: string };

export default function SetupWizardPage() {
  const router = useRouter();
  const [step, setStep] = useState(0);

  const [checks, setChecks] = useState<CheckResult[] | null>(null);
  const [checking, setChecking] = useState(false);

  const [db, setDb] = useState<DBConfig>({
    host: "127.0.0.1",
    port: "3306",
    name: "livechat",
    user: "root",
    password: "",
  });
  const [dbTestResult, setDbTestResult] = useState<{ ok: boolean; message: string } | null>(null);
  const [testingDb, setTestingDb] = useState(false);

  const [site, setSite] = useState<SiteConfig>({
    title: "LiveChat",
    timezone: "Asia/Kuala_Lumpur",
    baseUrl: "http://localhost:8080",
    uploadsPath: "./uploads",
    appPort: "8080",
    wsPort: "8081",
  });

  const [admin, setAdmin] = useState<AdminConfig>({
    fullName: "",
    email: "",
    password: "",
    confirmPassword: "",
  });

  const [finishing, setFinishing] = useState(false);
  const [finishError, setFinishError] = useState<string | null>(null);
  const [done, setDone] = useState(false);

  useEffect(() => {
    apiGet<{ setupComplete: boolean }>("/api/setup/status").then(({ setupComplete }) => {
      if (setupComplete) router.replace("/login");
    });
  }, [router]);

  async function runChecklist() {
    setChecking(true);
    try {
      const { checks } = await apiGet<{ checks: CheckResult[] }>("/api/setup/checklist");
      setChecks(checks);
    } catch {
      message.error("Could not reach the backend to run the checklist.");
    } finally {
      setChecking(false);
    }
  }

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- initial checklist run on mount.
    runChecklist();
  }, []);

  const checklistBlocked =
    checks === null || checks.some((c) => BLOCKING_CHECKS.has(c.name) && !c.pass);

  async function testDbConnection() {
    setTestingDb(true);
    setDbTestResult(null);
    try {
      const result = await apiPost<{ ok: boolean; message: string }>("/api/setup/db/test", db);
      setDbTestResult(result);
    } catch {
      setDbTestResult({ ok: false, message: "Request failed" });
    } finally {
      setTestingDb(false);
    }
  }

  async function handleFinish() {
    if (admin.password !== admin.confirmPassword) {
      setFinishError("Passwords do not match.");
      return;
    }
    if (admin.password.length < 8 || admin.password.length > 16 || !/[A-Z]/.test(admin.password) || !/[0-9]/.test(admin.password)) {
      setFinishError("Password must be 8-16 characters with at least one uppercase letter and one digit.");
      return;
    }
    setFinishing(true);
    setFinishError(null);
    try {
      await apiPost("/api/setup/finish", { db, site, admin });
      setDone(true);
    } catch (err) {
      setFinishError(err instanceof ApiError ? err.message : "Setup failed");
    } finally {
      setFinishing(false);
    }
  }

  if (done) {
    return (
      <div className="flex h-screen items-center justify-center">
        <Result
          status="success"
          title="Setup complete"
          subTitle="Your Super Admin account is ready."
          extra={
            <Button type="primary" onClick={() => router.replace("/login")}>
              Go to login
            </Button>
          }
        />
      </div>
    );
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-neutral-50 p-6 dark:bg-neutral-950">
      <Card style={{ width: 640 }}>
        <div className="mb-1 flex items-center gap-2 text-lg font-semibold tracking-tight">
          <MessageOutlined />
          LiveChat
        </div>
        <Typography.Title level={3}>Setup</Typography.Title>
        <Steps
          current={step}
          size="small"
          style={{ marginBottom: 24 }}
          items={[
            { title: "Environment" },
            { title: "Database" },
            { title: "Site" },
            { title: "Super Admin" },
            { title: "Finish" },
          ]}
        />

        {step === 0 && (
          <Space orientation="vertical" style={{ width: "100%" }}>
            {checking && checks === null ? (
              <Spin />
            ) : (
              <div className="flex flex-col gap-2">
                {(checks ?? []).map((c) => (
                  <div key={c.name} className="flex items-center gap-2 border-b border-black/5 py-2 last:border-0 dark:border-white/10">
                    {c.pass ? (
                      <CheckCircleFilled style={{ color: "#52c41a" }} />
                    ) : (
                      <CloseCircleFilled style={{ color: BLOCKING_CHECKS.has(c.name) ? "#ff4d4f" : "#faad14" }} />
                    )}
                    <span>{c.name}</span>
                    <Tag>{c.note}</Tag>
                  </div>
                ))}
              </div>
            )}
            <Button onClick={runChecklist} loading={checking}>
              Re-check
            </Button>
            {checklistBlocked && (
              <Alert
                type="warning"
                showIcon
                title="Fix the failing items above before continuing (Redis is optional)."
              />
            )}
            <Button type="primary" disabled={checklistBlocked} onClick={() => setStep(1)}>
              Next
            </Button>
          </Space>
        )}

        {step === 1 && (
          <Space orientation="vertical" style={{ width: "100%" }}>
            <LabeledInput label="Host" value={db.host} onChange={(v) => setDb({ ...db, host: v })} />
            <LabeledInput label="Port" value={db.port} onChange={(v) => setDb({ ...db, port: v })} />
            <LabeledInput label="Database name" value={db.name} onChange={(v) => setDb({ ...db, name: v })} />
            <LabeledInput label="User" value={db.user} onChange={(v) => setDb({ ...db, user: v })} />
            <LabeledInput
              label="Password"
              password
              value={db.password}
              onChange={(v) => setDb({ ...db, password: v })}
            />
            <Space>
              <Button onClick={testDbConnection} loading={testingDb}>
                Test Connection
              </Button>
              {dbTestResult && (
                <Tag color={dbTestResult.ok ? "success" : "error"}>{dbTestResult.message}</Tag>
              )}
            </Space>
            <Space>
              <Button onClick={() => setStep(0)}>Back</Button>
              <Button type="primary" disabled={!dbTestResult?.ok} onClick={() => setStep(2)}>
                Next
              </Button>
            </Space>
          </Space>
        )}

        {step === 2 && (
          <Space orientation="vertical" style={{ width: "100%" }}>
            <LabeledInput label="Site title" value={site.title} onChange={(v) => setSite({ ...site, title: v })} />
            <LabeledInput
              label="Timezone"
              value={site.timezone}
              onChange={(v) => setSite({ ...site, timezone: v })}
            />
            <LabeledInput
              label="Base URL"
              value={site.baseUrl}
              onChange={(v) => setSite({ ...site, baseUrl: v })}
            />
            <LabeledInput
              label="Uploads storage path"
              value={site.uploadsPath}
              onChange={(v) => setSite({ ...site, uploadsPath: v })}
            />
            <LabeledInput
              label="App port"
              value={site.appPort}
              onChange={(v) => setSite({ ...site, appPort: v })}
            />
            <LabeledInput
              label="WebSocket port"
              value={site.wsPort}
              onChange={(v) => setSite({ ...site, wsPort: v })}
            />
            <Space>
              <Button onClick={() => setStep(1)}>Back</Button>
              <Button type="primary" onClick={() => setStep(3)}>
                Next
              </Button>
            </Space>
          </Space>
        )}

        {step === 3 && (
          <Space orientation="vertical" style={{ width: "100%" }}>
            <LabeledInput
              label="Full Name"
              value={admin.fullName}
              onChange={(v) => setAdmin({ ...admin, fullName: v })}
            />
            <LabeledInput
              label="Email"
              value={admin.email}
              onChange={(v) => setAdmin({ ...admin, email: v })}
            />
            <LabeledInput
              label="Password"
              password
              value={admin.password}
              onChange={(v) => setAdmin({ ...admin, password: v })}
            />
            <LabeledInput
              label="Confirm password"
              password
              value={admin.confirmPassword}
              onChange={(v) => setAdmin({ ...admin, confirmPassword: v })}
            />
            <Typography.Text type="secondary">8-16 characters, at least one uppercase letter and one digit. Symbols optional.</Typography.Text>
            <Space>
              <Button onClick={() => setStep(2)}>Back</Button>
              <Button
                type="primary"
                disabled={!admin.fullName || !admin.email || !admin.password}
                onClick={() => setStep(4)}
              >
                Next
              </Button>
            </Space>
          </Space>
        )}

        {step === 4 && (
          <Space orientation="vertical" style={{ width: "100%" }}>
            <Descriptions column={1} bordered size="small">
              <Descriptions.Item label="Database">
                {db.user}@{db.host}:{db.port}/{db.name}
              </Descriptions.Item>
              <Descriptions.Item label="Site">{site.title}</Descriptions.Item>
              <Descriptions.Item label="Super Admin">
                {admin.fullName} ({admin.email})
              </Descriptions.Item>
            </Descriptions>
            {finishError && <Alert type="error" showIcon title={finishError} />}
            <Space>
              <Button onClick={() => setStep(3)}>Back</Button>
              <Button type="primary" loading={finishing} onClick={handleFinish}>
                Finish setup
              </Button>
            </Space>
          </Space>
        )}
      </Card>
    </div>
  );
}

function LabeledInput({
  label,
  value,
  onChange,
  password,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  password?: boolean;
}) {
  const Comp = password ? Input.Password : Input;
  return (
    <div>
      <Typography.Text strong>{label}</Typography.Text>
      <Comp value={value} onChange={(e) => onChange(e.target.value)} />
    </div>
  );
}
