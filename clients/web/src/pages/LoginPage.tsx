import { LoginForm } from '@/components/auth/LoginForm';

export default function LoginPage() {
  return (
    <div className="flex min-h-screen items-center justify-center bg-background px-4">
      <div className="w-full max-w-sm rounded-xl border border-border bg-card p-8">
        <div className="mb-6 text-center">
          <h1 className="text-2xl font-bold text-foreground">Constell</h1>
          <p className="mt-1 text-sm text-muted-foreground">Login to your account</p>
        </div>
        <LoginForm />
      </div>
    </div>
  );
}
