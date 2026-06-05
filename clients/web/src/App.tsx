import { BrowserRouter, Routes, Route } from 'react-router';
import { ClientProvider } from '@/hooks/useConstellClient';
import { AuthGuard } from '@/components/auth/AuthGuard';
import LoginPage from '@/pages/LoginPage';
import RegisterPage from '@/pages/RegisterPage';
import MainPage from '@/pages/MainPage';

export default function App() {
  return (
    <ClientProvider>
      <BrowserRouter>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/register" element={<RegisterPage />} />
          <Route
            path="/*"
            element={
              <AuthGuard>
                <MainPage />
              </AuthGuard>
            }
          />
        </Routes>
      </BrowserRouter>
    </ClientProvider>
  );
}
