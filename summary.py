import google.generativeai as genai
from flask import Flask, request, jsonify

app = Flask(__name__)

# Set up Gemini API
GOOGLE_API_KEY = "AIzaSyBfhT7EEYDL9x2cp0oqYicsxdNupmM2_CY"
genai.configure(api_key=GOOGLE_API_KEY)

@app.route('/summarize', methods=['POST'])
def summarize():
    try:
        data = request.json
        messages = data.get("messages", [])

        if not messages:
            return jsonify({"error": "No messages provided"}), 400

        conversation = " ".join(messages)

        # Custom prompt for Pros & Cons
        prompt = f"Summarize the following conversation in user perspective for example some users says like this while some says like that \n\n{conversation}\n\nFormat it like: plain text"

        # Use the correct method to generate text
        model = genai.GenerativeModel("gemini-1.5-pro-latest")
        response = model.generate_content(prompt)

        return jsonify({"summary": response.text})

    except Exception as e:
        return jsonify({"error": str(e)}), 500

if __name__ == '__main__':
    app.run(debug=True, port=6000)

