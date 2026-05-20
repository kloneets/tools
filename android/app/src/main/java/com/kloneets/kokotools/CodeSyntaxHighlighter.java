package com.kloneets.kokotools;

import java.util.ArrayList;
import java.util.Collections;
import java.util.List;

import io.noties.prism4j.AbsVisitor;
import io.noties.prism4j.Prism4j;
import io.noties.prism4j.annotations.PrismBundle;

@PrismBundle(
        include = {
                "clike",
                "css",
                "go",
                "java",
                "javascript",
                "json",
                "kotlin",
                "markdown",
                "markup",
                "python",
                "yaml"
        },
        grammarLocatorClassName = ".GeneratedPrismGrammarLocator"
)
public final class CodeSyntaxHighlighter {
    private static final Prism4j PRISM = new Prism4j(new GeneratedPrismGrammarLocator());

    private CodeSyntaxHighlighter() {
    }

    public static List<CodeToken> tokenize(String language, String code) {
        if (language == null || language.trim().isEmpty() || code == null || code.isEmpty()) {
            return Collections.emptyList();
        }

        try {
            final Prism4j.Grammar grammar = PRISM.grammar(language);
            if (grammar == null) {
                return Collections.emptyList();
            }

            final List<CodeToken> tokens = new ArrayList<>();
            final List<Prism4j.Node> nodes = PRISM.tokenize(code, grammar);
            new AbsVisitor() {
                private int offset = 0;

                @Override
                protected void visitText(Prism4j.Text text) {
                    offset += text.literal().length();
                }

                @Override
                protected void visitSyntax(Prism4j.Syntax syntax) {
                    final int start = offset;
                    visit(syntax.children());
                    final int end = offset;
                    if (end > start) {
                        tokens.add(new CodeToken(start, end, syntax.type()));
                    }
                }
            }.visit(nodes);
            return tokens;
        } catch (RuntimeException error) {
            return Collections.emptyList();
        }
    }

    public static final class CodeToken {
        public final int start;
        public final int end;
        public final String type;

        public CodeToken(int start, int end, String type) {
            this.start = start;
            this.end = end;
            this.type = type;
        }
    }
}
