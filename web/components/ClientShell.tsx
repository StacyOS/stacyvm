"use client";

import { useState, useEffect, ReactNode } from 'react';
import Link from 'next/link';
import { usePathname } from 'next/navigation';
import {
  LayoutDashboard,
  Box,
  FileCode2,
  Server,
  FlaskConical,
  Gauge,
  Settings as SettingsIcon,
  Menu,
  X,
  Building2,
} from 'lucide-react';
import { ToastProvider } from '../hooks/useToast';
import ToastContainer from './Toast';

const navItems = [
  { to: '/dashboard', label: 'Dashboard', icon: LayoutDashboard },
  { to: '/sandboxes', label: 'Sandboxes', icon: Box },
  { to: '/templates', label: 'Templates', icon: FileCode2 },
  { to: '/environments', label: 'Environments', icon: FlaskConical },
  { to: '/providers', label: 'Providers', icon: Server },
  { to: '/tenants', label: 'Tenants', icon: Building2 },
  { to: '/operations', label: 'Operations', icon: Gauge },
  { to: '/settings', label: 'Settings', icon: SettingsIcon },
];

export default function ClientShell({ children }: { children: ReactNode }) {
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [darkMode, setDarkMode] = useState(true);
  const pathname = usePathname();

  const [isIframe, setIsIframe] = useState(() => {
    if (typeof window !== 'undefined') {
      return window.self !== window.top;
    }
    return false;
  });

  useEffect(() => {
    if (isIframe) return;

    const stored = localStorage.getItem('stacyvm-theme');
    if (stored === 'light') {
      setDarkMode(false);
      document.documentElement.classList.remove('dark');
    } else {
      setDarkMode(true);
      document.documentElement.classList.add('dark');
    }
  }, []);

  if (isIframe) {
    return (
      <div className="flex h-screen items-center justify-center bg-navy-950 text-gray-500 p-6 text-center">
        <div>
          <img src="/stacy-mark-orange.png" alt="StacyVM Logo" className="w-12 h-12 mx-auto mb-4 opacity-40 object-contain" />
          <h2 className="text-lg font-medium text-gray-300">Live Preview Unavailable</h2>
          <p className="mt-2 text-sm text-gray-500">
            The sandbox preview domain could not be resolved or the Wails asset server intercepted the request.
          </p>
        </div>
      </div>
    );
  }

  const toggleTheme = () => {
    const next = !darkMode;
    setDarkMode(next);
    if (next) {
      document.documentElement.classList.add('dark');
      localStorage.setItem('stacyvm-theme', 'dark');
    } else {
      document.documentElement.classList.remove('dark');
      localStorage.setItem('stacyvm-theme', 'light');
    }
  };

  return (
    <ToastProvider>
      <div className="flex h-screen overflow-hidden bg-navy-800 text-gray-100 font-sans">
        {/* Mobile overlay */}
        {sidebarOpen && (
          <div
            className="fixed inset-0 z-30 bg-black/50 lg:hidden"
            onClick={() => setSidebarOpen(false)}
          />
        )}

        {/* Sidebar */}
        <aside
          className={`
            fixed inset-y-0 left-0 z-40 w-64 transform transition-transform duration-200 ease-in-out
            lg:relative lg:translate-x-0
            ${sidebarOpen ? 'translate-x-0' : '-translate-x-full'}
            bg-navy-900 border-r border-navy-700
            flex flex-col
          `}
        >
          {/* Logo */}
          <div className="flex items-center px-6 py-5 border-b border-navy-700">
            <img src="/stacy-logo-dark.png" alt="StacyVM Logo" className="h-9 w-auto object-contain" />
            <button
              className="ml-auto lg:hidden text-gray-400 hover:text-gray-200"
              onClick={() => setSidebarOpen(false)}
            >
              <X className="w-5 h-5" />
            </button>
          </div>

          {/* Navigation */}
          <nav className="flex-1 px-3 py-4 space-y-1 overflow-y-auto">
            {navItems.map(({ to, label, icon: Icon }) => {
              const isActive = pathname === to || pathname?.startsWith(to + '/');
              return (
                <Link
                  key={to}
                  href={to}
                  onClick={() => setSidebarOpen(false)}
                  className={`flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm font-medium transition-colors duration-150 ${
                    isActive
                      ? 'bg-primary-500/15 text-primary-400'
                      : 'text-gray-400 hover:bg-navy-700 hover:text-gray-200'
                  }`}
                >
                  <Icon className="w-5 h-5 flex-shrink-0" />
                  {label}
                </Link>
              );
            })}
          </nav>

          {/* Footer */}
          <div className="px-4 py-4 border-t border-navy-700">
            <div className="text-xs text-gray-600">
              StacyVM v0.1.0
            </div>
          </div>
        </aside>

        {/* Main content */}
        <div className="flex-1 flex flex-col min-w-0">
          {/* Top bar */}
          <header className="flex items-center gap-4 px-4 lg:px-6 py-3 border-b border-navy-700 bg-navy-800/80 backdrop-blur-sm">
            <button
              className="lg:hidden text-gray-400 hover:text-gray-200"
              onClick={() => setSidebarOpen(true)}
            >
              <Menu className="w-6 h-6" />
            </button>
            <div className="flex-1" />
            <button
              onClick={toggleTheme}
              className="btn-ghost text-sm"
              title={darkMode ? 'Switch to light mode' : 'Switch to dark mode'}
            >
              {darkMode ? 'Light' : 'Dark'}
            </button>
          </header>

          {/* Page content */}
          <main className="flex-1 overflow-y-auto p-4 lg:p-6">
            {children}
          </main>
        </div>
      </div>
      <ToastContainer />
    </ToastProvider>
  );
}
