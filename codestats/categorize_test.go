package main

import "testing"

func TestCategorizeFile(t *testing.T) {
	cases := []struct {
		path     string
		wantType FileType
		wantLang string
	}{
		// Maven/Gradle/sbt (shared src/main, src/test, resources layout).
		{"admin-service/src/main/java/Foo.java", MainCode, "java"},
		{"admin-service/src/test/java/FooTest.java", TestCode, "java"},
		{"admin-service/src/main/resources/application.yml", MainResources, "yaml"},
		{"admin-service/src/test/resources/application-test.yml", TestResources, "yaml"},
		{"admin-service/src/main/kotlin/Foo.kt", MainCode, "kotlin"},
		{"admin-service/src/test/scala/FooSpec.scala", TestCode, "scala"},
		{"pom.xml", Build, "xml"},
		{"admin-service/pom.xml", Build, "xml"},
		{"build.gradle", Build, "gradle"},
		{"build.gradle.kts", Build, "kotlin"},
		{"settings.gradle.kts", Build, "kotlin"},
		{"build.sbt", Build, "sbt"},
		{"project/Build.scala", MainCode, "scala"}, // documented sbt meta-build limitation

		// Go (colocated _test.go, no directory convention).
		{"cmd/server/main.go", MainCode, "go"},
		{"cmd/server/main_test.go", TestCode, "go"},
		{"internal/pkg/handler_test.go", TestCode, "go"},
		{"pkg/util/util.go", MainCode, "go"},

		// Python + Python/maturin (Rust).
		{"src/mypkg/app.py", MainCode, "python"},
		{"tests/test_app.py", TestCode, "python"},
		{"tests/app_test.py", TestCode, "python"},
		{"tests/conftest.py", TestCode, "python"},
		{"setup.py", Build, "python"},
		{"pyproject.toml", Build, "toml"},
		{"src/lib.rs", MainCode, "rust"},
		{"tests/integration.rs", TestCode, "rust"},
		{"Cargo.toml", Build, "toml"},

		// React/Angular/TypeScript.
		{"src/components/header/Header.tsx", MainCode, "typescript"},
		{"src/components/header/Header.test.tsx", TestCode, "typescript"},
		{"src/app/app.component.ts", MainCode, "typescript"},
		{"src/app/app.component.spec.ts", TestCode, "typescript"},
		{"src/app/app.component.html", MainCode, "html"},
		{"src/app/app.component.scss", MainCode, "scss"},
		{"src/assets/icons/logo.svg", Other, "other"}, // not a recognized code ext
		{"public/assets/themes/lara-dark/theme.css", MainResources, "css"},
		{"package.json", Build, "json"},
		{"package-lock.json", Build, "json"},
		{"tsconfig.json", Build, "json"},
		{"tsconfig.build.json", Build, "json"},
		{"angular.json", Build, "json"},

		// Playwright TS test-automation project.
		{"tests/ui/login.spec.ts", TestCode, "typescript"},
		{"pages/login-page.ts", MainCode, "typescript"},
		{"fixtures/user.ts", MainCode, "typescript"},

		// Ansible.
		{"playbooks/deploy.yml", MainCode, "yaml"},
		{"playbooks/templates/config.j2", MainResources, "jinja2"},
		{"accso/roles/gaeko_instance/tasks/main.yml", MainCode, "yaml"},
		{"accso/roles/gaeko_instance/templates/foo.j2", MainResources, "jinja2"},
		{"accso/roles/gaeko_instance/tests/test.yml", TestCode, "yaml"},
		{"accso/molecule/default/molecule.yml", TestCode, "yaml"},
		{"accso/plugins/filter/myfilter.py", MainCode, "python"},
		{"ansible.cfg", Build, "ini"},
		{".ansible-lint", Build, "other"},
		{"requirements.yml", Build, "yaml"},

		// Containers, compose, quadlet.
		{"Dockerfile", Build, "other"},
		{"admin-service/Dockerfile", Build, "other"},
		{"docker-compose.yml", Build, "yaml"},
		{"compose.yaml", Build, "yaml"},
		{"compose.override.yml", Build, "yaml"},
		{"podman-compose.prod.yaml", Build, "yaml"},
		{"quadlet/myapp.container", Build, "quadlet"},
		{"quadlet/myapp.volume", Build, "quadlet"},
		{"quadlet/myapp.network", Build, "quadlet"},
		{"quadlet/myapp.build", Build, "quadlet"},
		{"quadlet/myapp.image", Build, "quadlet"},

		// CI/build misc.
		{".gitlab-ci.yml", Build, "yaml"},
		{".github/workflows/ci.yml", Build, "yaml"},
		{".pre-commit-config.yaml", Build, "yaml"},

		// Scripts and docs stay their own categories regardless of path.
		{"scripts/deploy.sh", Scripts, "shell"},
		{"src/main/resources/scripts/run.sh", Scripts, "shell"},
		{"README.md", Documentation, "text"},
		{"docs/test-plan.md", Documentation, "text"},
	}

	for _, c := range cases {
		gotType, gotLang := categorizeFile(c.path)
		if gotType != c.wantType || gotLang != c.wantLang {
			t.Errorf("categorizeFile(%q) = (%q, %q), want (%q, %q)",
				c.path, gotType, gotLang, c.wantType, c.wantLang)
		}
	}
}

func TestIsCodeFile(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"src/App.tsx", true},
		{"src/app.jsx", true},
		{"styles/main.scss", true},
		{"templates/config.j2", true},
		{"build.gradle.kts", true},
		{"build.sbt", true},
		{"Cargo.toml", true},
		{"quadlet/myapp.container", true},
		{".ansible-lint", true},
		{"Dockerfile", true},
		{"compose.yaml", true},
		{"podman-compose.yml", true},
		{"src/assets/logo.svg", false},
		{"src/assets/font.woff2", false},
	}
	for _, c := range cases {
		if got := isCodeFile(c.path); got != c.want {
			t.Errorf("isCodeFile(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}
